package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/devileya/my-portfolio/internal/db"
	"github.com/devileya/my-portfolio/internal/jobs/browser"
	"github.com/devileya/my-portfolio/internal/secrets"
)

// A three-step application in a modal, written like the portals': a
// prefilled email, a required phone and CV, screening questions, a review
// step, then a confirmation. Next refuses to advance while required fields
// are empty.
const mockJob = `<!doctype html><html><body>
<h1>Senior Go Engineer</h1><button id="open">Easy Apply</button>
<div id="modal" role="dialog" style="display:none">
  <section data-step="1">
    <label for="email">Email address</label><input id="email" type="email" value="me@example.com">
    <label for="phone">Mobile phone number *</label><input id="phone" type="tel" required>
    <label for="cv">Upload resume</label><input id="cv" type="file" accept=".pdf" required>
    <p class="err" role="alert" hidden></p>
    <button class="next">Next</button>
  </section>
  <section data-step="2" hidden>
    <label for="years">How many years of experience do you have with Go?</label>
    <select id="years" required><option value="">Select an option</option><option>0-1</option><option>2-4</option><option>5+</option></select>
    <fieldset><legend>Are you comfortable working hybrid?</legend>
      <label><input type="radio" name="hybrid" value="y" required> Yes</label>
      <label><input type="radio" name="hybrid" value="n"> No</label>
    </fieldset>
    <label for="why">Why do you want to join Acme?</label><textarea id="why" required></textarea>
    <p class="err" role="alert" hidden></p>
    <button class="next">Review</button>
  </section>
  <section data-step="3" hidden>
    <p>Review your application</p>
    <button id="submit">Submit application</button>
  </section>
  <section data-step="4" hidden><p>Your application was sent to Acme.</p></section>
</div>
<script>
  document.getElementById('open').onclick = () => { document.getElementById('modal').style.display = 'block'; };
  const show = n => document.querySelectorAll('[data-step]').forEach(s => s.hidden = s.dataset.step != n);
  document.querySelectorAll('.next').forEach(b => b.onclick = () => {
    const step = b.closest('section');
    const missing = [...step.querySelectorAll('[required]')].filter(el =>
      el.type === 'radio' ? !step.querySelector('input[name="' + el.name + '"]:checked') : el.type === 'file' ? !el.files.length : !el.value);
    const err = step.querySelector('.err');
    if (missing.length) { err.hidden = false; err.textContent = 'Please complete the required fields.'; return; }
    window.__answers = Object.assign(window.__answers || {}, Object.fromEntries([...step.querySelectorAll('input:not([type=radio]),select,textarea')].map(e => [e.id, e.type === 'file' ? e.files[0].name : e.value])));
    show(Number(step.dataset.step) + 1);
  });
  document.getElementById('submit').onclick = () => { window.__submitted = true; show(4); };
</script></body></html>`

type mockPortal struct{ url string }

func (mockPortal) Name() string                                      { return "linkedin" }
func (mockPortal) CookieDomains() []string                           { return nil }
func (mockPortal) LoginURL() string                                  { return "" }
func (mockPortal) LoggedIn(context.Context, *rod.Page) (bool, error) { return true, nil }
func (mockPortal) Details(context.Context, *Env, *Listing) error     { return nil }
func (m mockPortal) Search(context.Context, *Env, Search, string, string, int) ([]Listing, error) {
	return nil, nil
}

func (m mockPortal) OpenApply(ctx context.Context, p *rod.Page, l Listing) ([]string, string, error) {
	if err := browser.Goto(p, m.url); err != nil {
		return nil, "", err
	}
	if _, ok := browser.ClickText(p, `^easy apply$`); !ok {
		return nil, "", fmt.Errorf("no apply button")
	}
	return []string{`[role="dialog"]`}, "", nil
}

// fakeModel answers like a model would: phone from the facts, an option from
// the list, a sentence for free text.
func fakeModel(t *testing.T, unknown *atomic.Bool) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []struct{ Content string } `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		user := body.Messages[len(body.Messages)-1].Content
		raw := user[strings.Index(user, "<fields>")+8 : strings.Index(user, "</fields>")]
		var fields []browser.Field
		if err := json.Unmarshal([]byte(raw), &fields); err != nil {
			t.Errorf("fields: %v", err)
		}
		var answers []map[string]string
		for _, f := range fields {
			v := "I build Go services and want to work on Acme's platform."
			switch {
			case strings.Contains(f.Label, "phone"):
				v = "+62 812 0000 0000"
			case len(f.Options) > 0:
				v = f.Options[len(f.Options)-1]
				if f.Kind == "radio" {
					v = f.Options[0]
				}
			}
			conf := "high"
			if unknown.Load() && strings.Contains(f.Label, "years") {
				v, conf = "", "unknown"
			}
			answers = append(answers, map[string]string{"id": f.ID, "value": v, "confidence": conf, "source": "inferred"})
		}
		reply, _ := json.Marshal(map[string]any{"answers": answers, "coverLetter": ""})
		out, _ := json.Marshal(map[string]any{"choices": []map[string]any{{"message": map[string]string{"content": string(reply)}}}})
		_, _ = w.Write(out)
	}))
}

// TestApplyFlow drives the real form engine through the mock form in Chrome.
// It needs PF_TEST_DATABASE_URL (a disposable database) and Chrome.
func TestApplyFlow(t *testing.T) {
	dsn := os.Getenv("PF_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set PF_TEST_DATABASE_URL to a disposable database to run")
	}
	if _, ok := launcher.LookPath(); !ok {
		t.Skip("Chrome not found")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err := db.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}

	var unknown atomic.Bool
	model := fakeModel(t, &unknown)
	defer model.Close()
	repo := NewRepo(pool, secrets.New(strings.Repeat("k", 32)))
	prov, err := repo.SaveProvider(ctx, Provider{Label: "fake", Kind: "openai", BaseURL: model.URL, Model: "m", APIKey: "sk-fake-key"})
	if err != nil {
		t.Fatal(err)
	}
	set, _ := repo.Settings(ctx)
	set.ScoreProviderID = &prov.ID
	if err := repo.SaveSettings(ctx, set); err != nil {
		t.Fatal(err)
	}
	svc := &Service{Repo: repo}

	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(mockJob)) }))
	defer site.Close()
	cv := filepath.Join(t.TempDir(), "Arif_CV.pdf")
	_ = os.WriteFile(cv, []byte("%PDF-1.4\n%%EOF\n"), 0o600)
	app := Applicant{Text: "Name: Test\nphone: +62 812 0000 0000\n", CVPath: cv, CVName: "Arif_CV.pdf", Settings: set}

	b := browser.New(browser.Options{Dir: t.TempDir(), Headless: true})
	defer b.Close()

	for _, dry := range []bool{true, false} {
		p, err := b.Page(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		res := svc.apply(ctx, p, mockPortal{url: site.URL}, Listing{Title: "Senior Go Engineer", Company: "Acme"}, app, dry)
		var submitted bool
		var answers map[string]string
		_ = browser.Eval(p, `() => !!window.__submitted`, &submitted)
		_ = browser.Eval(p, `() => window.__answers || {}`, &answers)
		_ = p.Close()

		want := "submitted"
		if dry {
			want = "dry_run"
		}
		if res.Status != want {
			t.Fatalf("dry=%v: status %q (%s), want %q; pending %+v", dry, res.Status, res.Note, want, res.Pending)
		}
		if submitted == dry {
			t.Fatalf("dry=%v: submit pressed = %v", dry, submitted)
		}
		if answers["phone"] != "+62 812 0000 0000" || answers["years"] != "5+" || answers["cv"] != "Arif_CV.pdf" || answers["email"] != "me@example.com" {
			t.Fatalf("dry=%v: form got %v", dry, answers)
		}
		if len(res.Shot) == 0 {
			t.Fatalf("dry=%v: no screenshot", dry)
		}
	}

	// A required question the model can't answer stops for the user, with
	// the question and its options, instead of guessing.
	unknown.Store(true)
	p, err := b.Page(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	res := svc.apply(ctx, p, mockPortal{url: site.URL}, Listing{Title: "Senior Go Engineer", Company: "Acme"}, app, false)
	if res.Status != "needs_input" || len(res.Pending) != 1 || len(res.Pending[0].Options) != 3 ||
		!strings.Contains(res.Pending[0].Question, "years of experience") {
		t.Fatalf("unknown answer: status %q, pending %+v", res.Status, res.Pending)
	}
	var submitted bool
	_ = browser.Eval(p, `() => !!window.__submitted`, &submitted)
	if submitted {
		t.Fatal("submitted despite an unanswered question")
	}
}
