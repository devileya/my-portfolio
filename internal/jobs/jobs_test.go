package jobs

import (
	"strings"
	"testing"
	"time"

	"github.com/devileya/my-portfolio/internal/jobs/browser"
)

func TestFingerprintMatchesAcrossPortals(t *testing.T) {
	a := Fingerprint("PT Kredivo Group Indonesia", "Backend Engineer - Golang (SDE II) [Remote]")
	b := Fingerprint("Kredivo Group", "Backend Engineer – Golang")
	if a != b {
		t.Fatalf("fingerprints differ:\n%q\n%q", a, b)
	}
	if Fingerprint("Acme", "Frontend Engineer") == Fingerprint("Acme", "Backend Engineer") {
		t.Fatal("different titles share a fingerprint")
	}
}

func TestQuestionKey(t *testing.T) {
	for _, q := range []string{"Years of experience with Go? *", "years of experience with go", "Years of Experience with Go (required)"} {
		if got := QuestionKey(q); got != "years of experience with go" {
			t.Errorf("QuestionKey(%q) = %q", q, got)
		}
	}
}

func TestHTMLText(t *testing.T) {
	got := HTMLText("<p>We build <strong>things</strong>.</p><ul><li>Go</li><li>Postgres</li></ul><script>x()</script>")
	want := "We build things.\n\n• Go\n• Postgres"
	if got != want {
		t.Fatalf("HTMLText:\n%q\nwant\n%q", got, want)
	}
}

func TestMatchesAnyIsWholeWord(t *testing.T) {
	if matchesAny("Senior Golang Engineer", []string{"go"}) != "" {
		t.Error("“go” should not match inside “Golang”")
	}
	if matchesAny("PHP Developer (Laravel)", []string{"php"}) != "php" {
		t.Error("“php” should match")
	}
}

func TestNextRunUsesTimezone(t *testing.T) {
	after := time.Date(2026, 10, 18, 2, 30, 0, 0, time.UTC) // 09:30 in Jakarta
	next, err := NextRun("0 9,13,17 * * *", "Asia/Jakarta", after)
	if err != nil {
		t.Fatal(err)
	}
	if want := time.Date(2026, 10, 18, 6, 0, 0, 0, time.UTC); !next.Equal(want) {
		t.Fatalf("next = %s, want %s", next.UTC(), want)
	}
	if _, err := NextRun("every day", "UTC", after); err == nil {
		t.Fatal("expected an error for a bad schedule")
	}
}

func TestFitsOptions(t *testing.T) {
	f := browser.Field{Kind: "select", Options: []string{"Yes", "No"}}
	if !fits(f, "yes") || fits(f, "Maybe") {
		t.Fatal("option matching is wrong")
	}
	if !fits(browser.Field{Kind: "text"}, "anything") {
		t.Fatal("free text always fits")
	}
}

func TestPickButtonPrefersSubmit(t *testing.T) {
	b, ok := pickButton([]browser.Button{{Kind: "other"}, {Kind: "next", Text: "Next"}, {Kind: "submit", Text: "Submit application"}})
	if !ok || b.Kind != "submit" {
		t.Fatalf("picked %+v", b)
	}
	if _, ok := pickButton([]browser.Button{{Kind: "other"}}); ok {
		t.Fatal("no usable button should be reported")
	}
}

func TestSuccessText(t *testing.T) {
	for _, s := range []string{"Your application was sent to Kredivo", "Lamaran kamu berhasil dikirim!", "Thank you for applying"} {
		if !successText.MatchString(s) {
			t.Errorf("not recognised as success: %q", s)
		}
	}
	if successText.MatchString("Submit application") {
		t.Error("a button label is not success")
	}
}

func TestKalibrrListing(t *testing.T) {
	j := kalibrrJob{ID: 7, Name: "Golang Developer", Slug: "golang-developer-21", Tenure: "Full time", IsWFH: true}
	j.Company.Code, j.Company.Name = "mobius-digital", "Mobius Digital"
	l := j.listing()
	if l.URL != "https://www.kalibrr.com/c/mobius-digital/jobs/7/golang-developer-21" || l.WorkMode != "remote" || l.JobType != "full_time" {
		t.Fatalf("unexpected listing %+v", l)
	}
	if l.EasyApply == nil || !*l.EasyApply {
		t.Fatal("no redirect URL means apply on Kalibrr")
	}
}

func TestLinkedInCardParsing(t *testing.T) {
	card := `<li><div class="base-card" data-entity-urn="urn:li:jobPosting:4440204415">
		<h3 class="base-search-card__title">
			Backend Engineer - Golang (SDE II)
		</h3>
		<h4 class="base-search-card__subtitle"><a href="x">
			Kredivo Group
		</a></h4>
		<span class="job-search-card__location">
			Jakarta Metropolitan Area
		</span>
		<time class="job-search-card__listdate" datetime="2026-10-09">1 week ago</time></div></li>`
	m := liCard.FindAllStringSubmatch(card, -1)
	if len(m) != 1 || m[0][1] != "4440204415" {
		t.Fatalf("card not found: %v", m)
	}
	if got := cleanHTML(first(liTitle, m[0][0])); got != "Backend Engineer - Golang (SDE II)" {
		t.Errorf("title %q", got)
	}
	if got := cleanHTML(first(liCompany, m[0][0])); got != "Kredivo Group" {
		t.Errorf("company %q", got)
	}
	if got := first(liDate, m[0][0]); got != "2026-10-09" {
		t.Errorf("date %q", got)
	}
	if !strings.Contains(cleanHTML(first(liLocation, m[0][0])), "Jakarta") {
		t.Error("location missing")
	}
}
