package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/go-rod/rod"

	"github.com/devileya/my-portfolio/internal/ai"
	"github.com/devileya/my-portfolio/internal/jobs/browser"
)

// applyResult is what one attempt produced; the worker records it.
type applyResult struct {
	Status   string // submitted | dry_run | needs_input | external | failed
	Note     string
	External string
	Answers  []Answer
	Pending  []Question
	Cover    string
	Shot     []byte
	Steps    int
}

var successText = regexp.MustCompile(`(?i)(your application (was|has been) (sent|submitted)|application (sent|submitted)( successfully)?|you('ve| have) (successfully )?applied|applied successfully|lamaran (anda |kamu )?(berhasil |telah |sudah )?(dikirim|terkirim)|berhasil melamar|thank you for applying|terima kasih (telah|sudah) melamar)`)

const maxSteps = 14

// apply runs one application in page p: open the form, then read, answer and
// advance each step until it's submitted or something needs the user.
func (s *Service) apply(ctx context.Context, p *rod.Page, portal Portal, l Listing, app Applicant, dryRun bool) applyResult {
	res := applyResult{}
	known := pageIDs(p)
	roots, external, err := portal.OpenApply(ctx, p, l)
	switch {
	case errors.Is(err, ErrAlreadyApplied):
		res.Status, res.Note = "submitted", "The portal shows this job as already applied."
		return res
	case err != nil:
		return s.failWith(p, res, err)
	case external != "":
		res.Status, res.External = "external", external
		res.Note = "Applying happens on the company's own site."
		return res
	}
	// Some flows (Indeed Apply) continue in a new tab.
	p = followNewTab(p, known)

	client, err := s.AI(ctx, "apply")
	if err != nil {
		return s.failWith(p, res, err)
	}
	bank, err := s.Repo.AnswerBank(ctx)
	if err != nil {
		return s.failWith(p, res, err)
	}

	var lastSig string
	repeats := 0
	for res.Steps = 1; res.Steps <= maxSteps; res.Steps++ {
		if ch := browser.Challenge(p); ch != "" {
			res.Shot, _ = browser.Screenshot(p)
			return s.failWith(p, res, &ChallengeError{Portal: l.Portal, What: ch})
		}
		step, err := browser.ReadStep(p, roots...)
		if err != nil {
			return s.failWith(p, res, fmt.Errorf("read the form: %w", err))
		}
		if successText.MatchString(step.Text) || successText.MatchString(browser.PageText(p, 4000)) {
			res.Status = "submitted"
			res.Shot, _ = browser.Screenshot(p)
			return res
		}

		// The same filled step came back: the form isn't accepting our answers.
		sig := stepSignature(step)
		if sig == lastSig {
			repeats++
		} else {
			repeats = 0
		}
		lastSig = sig
		if repeats >= 1 {
			res.Shot, _ = browser.Screenshot(p)
			res.Status = "needs_input"
			res.Note = "The form didn't accept the answers"
			if len(step.Errors) > 0 {
				res.Note += ": " + strings.Join(step.Errors, "; ")
			}
			res.Pending = append(res.Pending, questionsFromErrors(step)...)
			return res
		}

		pending, answers, cover, err := s.answerStep(ctx, client, p, step, l, app, bank)
		res.Answers = append(res.Answers, answers...)
		if cover != "" {
			res.Cover = cover
		}
		if err != nil {
			return s.failWith(p, res, err)
		}
		if len(pending) > 0 {
			res.Shot, _ = browser.Screenshot(p)
			res.Status, res.Pending = "needs_input", pending
			res.Note = fmt.Sprintf("%d question(s) need your answer.", len(pending))
			return res
		}

		btn, ok := pickButton(step.Buttons)
		if !ok {
			// Some pages render their form after a moment.
			browser.Pause(ctx, 2*time.Second, 3*time.Second)
			if step2, _ := browser.ReadStep(p, roots...); len(step2.Buttons) > 0 {
				if btn, ok = pickButton(step2.Buttons); ok {
					step = step2
				}
			}
		}
		if !ok {
			res.Shot, _ = browser.Screenshot(p)
			return s.failWith(p, res, errors.New("couldn't find a Next or Submit button on the form"))
		}
		if btn.Kind == "submit" && dryRun {
			res.Shot, _ = browser.Screenshot(p)
			res.Status = "dry_run"
			res.Note = "Dry run: the form was filled up to the final Submit, which was not pressed."
			return res
		}
		browser.Pause(ctx, 900*time.Millisecond, 2200*time.Millisecond)
		if err := browser.Click(p, btn); err != nil {
			return s.failWith(p, res, err)
		}
		browser.Pause(ctx, 2*time.Second, 4*time.Second)
		p = followNewTab(p, known)

		if btn.Kind == "submit" {
			// Success text can take a moment, or the modal closes outright.
			for i := 0; i < 5; i++ {
				text := browser.PageText(p, 5000)
				if successText.MatchString(text) {
					res.Status = "submitted"
					res.Shot, _ = browser.Screenshot(p)
					return res
				}
				browser.Pause(ctx, time.Second, 2*time.Second)
			}
			if after, err := browser.ReadStep(p, roots...); err == nil && len(after.Fields) == 0 && !hasKind(after.Buttons, "submit") {
				res.Status, res.Note = "submitted", "Submitted; the portal showed no confirmation text."
				res.Shot, _ = browser.Screenshot(p)
				return res
			}
		}
	}
	res.Shot, _ = browser.Screenshot(p)
	return s.failWith(p, res, fmt.Errorf("gave up after %d form steps", maxSteps))
}

func (s *Service) failWith(p *rod.Page, res applyResult, err error) applyResult {
	if res.Shot == nil && p != nil {
		res.Shot, _ = browser.Screenshot(p)
	}
	res.Status = "failed"
	res.Note = err.Error()
	var ch *ChallengeError
	if errors.As(err, &ch) {
		res.Status = "challenge"
	}
	return res
}

func pageIDs(p *rod.Page) map[string]bool {
	ids := map[string]bool{}
	if pages, err := p.Browser().Pages(); err == nil {
		for _, pg := range pages {
			ids[string(pg.TargetID)] = true
		}
	}
	return ids
}

// followNewTab switches to a tab opened since known was taken (and adds it
// to known), or keeps p.
func followNewTab(p *rod.Page, known map[string]bool) *rod.Page {
	pages, err := p.Browser().Pages()
	if err != nil {
		return p
	}
	for _, pg := range pages {
		id := string(pg.TargetID)
		if known[id] {
			continue
		}
		known[id] = true
		if info, err := pg.Info(); err == nil && info.Type == "page" && info.URL != "about:blank" {
			pg = pg.Context(p.GetContext())
			_, _ = pg.Activate()
			_ = pg.Timeout(20 * time.Second).WaitLoad()
			return pg
		}
	}
	return p
}

func pickButton(buttons []browser.Button) (browser.Button, bool) {
	for _, kind := range []string{"submit", "review", "next"} {
		for _, b := range buttons {
			if b.Kind == kind {
				return b, true
			}
		}
	}
	return browser.Button{}, false
}

func hasKind(buttons []browser.Button, kind string) bool {
	for _, b := range buttons {
		if b.Kind == kind {
			return true
		}
	}
	return false
}

func stepSignature(st browser.Step) string {
	var b strings.Builder
	for _, f := range st.Fields {
		b.WriteString(f.Label + "=" + f.Value + ";")
	}
	b.WriteString(strings.Join(st.Errors, "|"))
	return b.String()
}

func questionsFromErrors(st browser.Step) []Question {
	var out []Question
	for _, f := range st.Fields {
		if f.Required && strings.TrimSpace(f.Value) == "" && f.Kind != "file" {
			out = append(out, Question{Key: QuestionKey(f.Label), Question: f.Label, Kind: f.Kind, Options: f.Options})
		}
	}
	return out
}

// ── answering ────────────────────────────────────────────────────────────────

var (
	resumeLabel = regexp.MustCompile(`(?i)\b(cv|resume|résumé|riwayat hidup|curriculum)\b`)
	coverLabel  = regexp.MustCompile(`(?i)cover ?letter|surat lamaran|motivation`)
)

type fieldAnswer struct {
	ID         string `json:"id"`
	Value      string `json:"value"`
	Confidence string `json:"confidence"`
	Source     string `json:"source"`
}

const answerSystem = `You fill in job application forms on behalf of one candidate. You get the candidate's profile, saved answers, the job, and the form fields on the current step.

For each field, give the value to enter:
- Use only facts from the profile, the facts list and the saved answers. Never invent certifications, licences, degrees, employers, legal or visa status, or personal data. If a required fact isn't there, return value "" with confidence "unknown".
- Years of experience with a skill: estimate from the timeline and skills (count overlapping roles once), as a whole number.
- Yes/No about a skill or tool clearly in the profile: "Yes". About something absent: "No" only when the field is a simple self-assessment; otherwise unknown.
- Salary questions: use expectedSalary / currentSalary from the facts, digits only, in the currency the form implies. If absent, unknown.
- select and radio: the value must be exactly one of the listed options.
- checkbox: "Yes" or "No". Agree to terms/privacy/data-processing checkboxes ("Yes"); leave marketing opt-ins "No".
- number: digits only. tel: the phone from the facts. email: the candidate's email.
- Free-text motivation or cover-letter fields: 3-5 sentences, specific to this job and the candidate's real experience, in the language the job is written in, no clichés, no placeholders. Confidence "high".
- Fields that already hold a correct value: return the same value.
Fields:
- answers: [{"id": field id, "value": string, "confidence": "high" | "low" | "unknown", "source": "profile" | "saved" | "inferred" | "written"}]
- coverLetter: the text you wrote for a cover-letter field, or ""`

// answerStep fills every field it can and returns the required questions it
// could not answer. Saved answers are used directly; the rest go to the model
// in one call per step.
func (s *Service) answerStep(ctx context.Context, c ai.Client, p *rod.Page, step browser.Step, l Listing, app Applicant, bank map[string]SavedAnswer) ([]Question, []Answer, string, error) {
	var pending []Question
	var answers []Answer
	var ask []browser.Field
	var usedKeys []string

	for _, f := range step.Fields {
		if f.Kind == "file" {
			if f.Value != "" {
				continue
			}
			switch {
			case resumeLabel.MatchString(f.Label) || (f.Label == "" && strings.Contains(f.Accept, "pdf")) || (!coverLabel.MatchString(f.Label) && f.Required):
				if app.CVPath == "" {
					return nil, answers, "", errors.New("the form wants a CV, but none is uploaded in the dashboard")
				}
				if err := browser.Fill(p, f, app.CVPath); err != nil {
					return nil, answers, "", fmt.Errorf("attach CV: %w", err)
				}
				answers = append(answers, Answer{Question: orLabel(f.Label, "CV"), Answer: app.CVName, Source: "profile"})
				browser.Pause(ctx, 1500*time.Millisecond, 3*time.Second)
			case f.Required:
				pending = append(pending, Question{Key: QuestionKey(f.Label), Question: f.Label + " (file upload)", Kind: "file"})
			}
			continue
		}
		// Portals prefill name/email/phone from the profile; keep those.
		if strings.TrimSpace(f.Value) != "" && f.Kind != "checkbox" && f.Kind != "select" && f.Kind != "radio" {
			continue
		}
		if (f.Kind == "select" || f.Kind == "radio") && f.Value != "" {
			continue
		}
		if a, ok := bank[QuestionKey(f.Label)]; ok && a.Answer != "" && fits(f, a.Answer) {
			if err := browser.Fill(p, f, a.Answer); err == nil {
				answers = append(answers, Answer{Question: f.Label, Answer: a.Answer, Source: "saved"})
				usedKeys = append(usedKeys, a.Key)
				browser.Pause(ctx, 300*time.Millisecond, 900*time.Millisecond)
				continue
			}
		}
		if f.Kind == "checkbox" && !f.Required {
			continue // optional checkboxes are usually marketing opt-ins
		}
		ask = append(ask, f)
	}
	s.Repo.MarkAnswersUsed(ctx, usedKeys)
	if len(ask) == 0 {
		return pending, answers, "", nil
	}

	var saved strings.Builder
	for _, a := range bank {
		fmt.Fprintf(&saved, "Q: %s\nA: %s\n", a.Question, a.Answer)
	}
	fieldsJSON, _ := json.Marshal(ask)
	user := fmt.Sprintf("<candidate>\n%s</candidate>\n\n<saved_answers>\n%s</saved_answers>\n\n<cover_letter_style>%s</cover_letter_style>\n\n<job>\nTitle: %s\nCompany: %s\nLocation: %s\n\n%s\n</job>\n\n<fields>\n%s\n</fields>",
		app.Text, saved.String(), app.Settings.CoverLetterStyle, l.Title, l.Company, l.Location, Clip(l.Description, 6000), fieldsJSON)

	var reply struct {
		Answers     []fieldAnswer `json:"answers"`
		CoverLetter string        `json:"coverLetter"`
	}
	if err := ai.CompleteJSON(ctx, c, answerSystem, user, &reply); err != nil {
		return nil, answers, "", fmt.Errorf("AI form answers: %w", err)
	}
	byID := map[string]fieldAnswer{}
	for _, a := range reply.Answers {
		byID[a.ID] = a
	}

	for _, f := range ask {
		a, ok := byID[f.ID]
		val := strings.TrimSpace(a.Value)
		usable := ok && val != "" && a.Confidence != "unknown" && fits(f, val)
		// Low-confidence factual answers on required fields go to the user.
		if usable && a.Confidence == "low" && f.Required && f.Kind != "textarea" {
			usable = false
		}
		if !usable {
			if f.Required {
				pending = append(pending, Question{Key: QuestionKey(f.Label), Question: orLabel(f.Label, "Unlabelled field"), Kind: f.Kind, Options: f.Options})
			}
			continue
		}
		if err := browser.Fill(p, f, val); err != nil {
			if f.Required {
				pending = append(pending, Question{Key: QuestionKey(f.Label), Question: f.Label, Kind: f.Kind, Options: f.Options})
			}
			continue
		}
		answers = append(answers, Answer{Question: f.Label, Answer: val, Source: orLabel(a.Source, "inferred")})
		// Remember short factual answers as suggestions to approve later.
		if a.Source == "inferred" && f.Kind != "textarea" && len(val) <= 120 && f.Label != "" {
			_, _ = s.Repo.SaveAnswer(ctx, SavedAnswer{Question: f.Label, Answer: val, Source: "ai", Approved: false})
		}
		browser.Pause(ctx, 400*time.Millisecond, 1200*time.Millisecond)
	}
	return pending, answers, reply.CoverLetter, nil
}

// fits reports whether value is acceptable for the field's kind.
func fits(f browser.Field, value string) bool {
	if len(f.Options) > 0 && (f.Kind == "select" || f.Kind == "radio") {
		v := strings.ToLower(strings.TrimSpace(value))
		for _, o := range f.Options {
			o = strings.ToLower(o)
			if o == v || strings.Contains(o, v) {
				return true
			}
		}
		return false
	}
	return true
}

func orLabel(s, fallback string) string {
	if strings.TrimSpace(s) == "" {
		return fallback
	}
	return s
}
