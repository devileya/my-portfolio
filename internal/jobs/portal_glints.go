package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/go-rod/rod"

	"github.com/devileya/my-portfolio/internal/jobs/browser"
)

// Glints: a firewall blocks plain HTTP, so searching opens the explore page
// in the browser and reads the server-rendered results (__NEXT_DATA__).
type glints struct{}

func (glints) Name() string            { return "glints" }
func (glints) CookieDomains() []string { return []string{"glints.com"} }
func (glints) LoginURL() string        { return "https://glints.com/id/login" }

func (glints) LoggedIn(ctx context.Context, p *rod.Page) (bool, error) {
	if err := browser.Goto(p, "https://glints.com/id/opportunities/jobs/explore"); err != nil {
		return false, err
	}
	return !loggedOutHeader(p), nil
}

var glintsMode = map[string]string{"remote": "REMOTE", "hybrid": "HYBRID", "onsite": "ONSITE"}
var glintsType = map[string]string{"full_time": "FULL_TIME", "contract": "CONTRACT", "internship": "INTERNSHIP", "part_time": "PART_TIME", "freelance": "PROJECT_BASED"}

type glintsJob struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	WorkMode   string `json:"workArrangementOption"`
	Type       string `json:"type"`
	CreatedAt  string `json:"createdAt"`
	UpdatedAt  string `json:"updatedAt"`
	ShowSalary bool   `json:"shouldShowSalary"`
	IsApplied  bool   `json:"isApplied"`
	Company    struct{ Name, BrandName string }
	City       *struct{ Name string }
	Country    *struct{ Name string }
	Location   *struct{ FormattedName string }
	MinYears   *int `json:"minYearsOfExperience"`
	MaxYears   *int `json:"maxYearsOfExperience"`
	Salaries   []struct {
		Mode     string  `json:"salaryMode"`
		Min      float64 `json:"minAmount"`
		Max      float64 `json:"maxAmount"`
		Currency string  `json:"CurrencyCode"`
	} `json:"salaries"`
	// Decoded leniently: its shape isn't documented and varies.
	Skills json.RawMessage `json:"skills"`
}

func (glints) Search(ctx context.Context, env *Env, s Search, keyword, location string, limit int) ([]Listing, error) {
	q := url.Values{"keyword": {keyword}, "country": {"ID"}, "locationName": {"All Cities/Provinces"}}
	if v := joinCodes(s.WorkModes, glintsMode); v != "" {
		q.Set("workArrangementOptions", v)
	}
	if v := joinCodes(s.JobTypes, glintsType); v != "" {
		q.Set("jobTypes", v)
	}
	switch d := s.PostedWithinDays; {
	case d > 0 && d <= 1:
		q.Set("lastUpdated", "PAST_24_HOURS")
	case d > 0 && d <= 7:
		q.Set("lastUpdated", "PAST_WEEK")
	case d > 0 && d <= 31:
		q.Set("lastUpdated", "PAST_MONTH")
	}
	p, err := env.Page(ctx)
	if err != nil {
		return nil, err
	}
	defer p.Close()
	if err := browser.Goto(p, "https://glints.com/id/opportunities/jobs/explore?"+q.Encode()); err != nil {
		return nil, err
	}
	if ch := browser.Challenge(p); ch != "" {
		return nil, &ChallengeError{Portal: "glints", What: ch}
	}
	var raw string
	if err := browser.Eval(p, `() => { const n = document.getElementById('__NEXT_DATA__'); return n ? n.textContent : ''; }`, &raw); err != nil || raw == "" {
		return nil, fmt.Errorf("glints: the results page changed (no __NEXT_DATA__)")
	}
	var data struct {
		Props struct {
			PageProps struct {
				InitialJobs struct {
					Jobs []glintsJob `json:"jobsInPage"`
				} `json:"initialJobs"`
			} `json:"pageProps"`
		} `json:"props"`
	}
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		return nil, fmt.Errorf("glints: unexpected results data")
	}
	var out []Listing
	for _, j := range data.Props.PageProps.InitialJobs.Jobs {
		if j.IsApplied {
			continue
		}
		l := j.listing()
		if location != "" && !strings.EqualFold(location, "remote") && l.WorkMode != "remote" &&
			!strings.Contains(strings.ToLower(l.Location), strings.ToLower(location)) && !strings.EqualFold(location, "indonesia") {
			continue
		}
		out = append(out, l)
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (j glintsJob) listing() Listing {
	company := j.Company.BrandName
	if company == "" {
		company = j.Company.Name
	}
	l := Listing{
		Portal: "glints", ExternalID: j.ID, Title: j.Title, Company: company,
		URL:      fmt.Sprintf("https://glints.com/id/opportunities/jobs/%s/%s", slugify(j.Title), j.ID),
		WorkMode: strings.ToLower(j.WorkMode),
		JobType:  normJobType(strings.ReplaceAll(j.Type, "_", " ")),
	}
	var loc []string
	if j.Location != nil {
		loc = append(loc, j.Location.FormattedName)
	}
	if j.City != nil {
		loc = append(loc, j.City.Name)
	}
	if j.Country != nil {
		loc = append(loc, j.Country.Name)
	}
	l.Location = joinNonEmpty(", ", loc...)
	if j.ShowSalary && len(j.Salaries) > 0 {
		s := j.Salaries[0]
		l.Salary = fmt.Sprintf("%s %s – %s / %s", s.Currency, money(s.Min), money(s.Max), strings.ToLower(s.Mode))
	}
	for _, ts := range []string{j.UpdatedAt, j.CreatedAt} {
		if t, err := time.Parse(time.RFC3339Nano, ts); err == nil {
			l.PostedAt = &t
			break
		}
	}
	var skills []string
	var raw []struct {
		Name  string
		Skill struct{ Name string }
	}
	if json.Unmarshal(j.Skills, &raw) == nil {
		for _, sk := range raw {
			if n := sk.Skill.Name + sk.Name; n != "" {
				skills = append(skills, n)
			}
		}
	}
	var b strings.Builder
	if j.MinYears != nil {
		fmt.Fprintf(&b, "Experience: %d", *j.MinYears)
		if j.MaxYears != nil {
			fmt.Fprintf(&b, "–%d", *j.MaxYears)
		}
		b.WriteString(" years\n")
	}
	if len(skills) > 0 {
		fmt.Fprintf(&b, "Skills: %s\n", strings.Join(skills, ", "))
	}
	l.Description = b.String()
	return l
}

func (glints) Details(ctx context.Context, env *Env, l *Listing) error {
	p, err := env.Page(ctx)
	if err != nil {
		return err
	}
	defer p.Close()
	if err := browser.Goto(p, l.URL); err != nil {
		return err
	}
	if ch := browser.Challenge(p); ch != "" {
		return &ChallengeError{Portal: "glints", What: ch}
	}
	var d struct {
		Text     string `json:"text"`
		External bool   `json:"external"`
	}
	_ = browser.Eval(p, `() => {
		const pick = document.querySelector('[class*="DescriptionContainer" i], [class*="JobDescription" i], [aria-label*="description" i]');
		const text = (pick || document.querySelector('main') || document.body).innerText;
		const ext = [...document.querySelectorAll('a, button')].some(el => /lamar di situs|apply on (company )?website|situs perusahaan/i.test(el.innerText || ''));
		return { text: text.slice(0, 12000), external: ext };
	}`, &d)
	if strings.TrimSpace(d.Text) != "" {
		l.Description = strings.TrimSpace(l.Description + "\n" + d.Text)
	}
	l.EasyApply = boolPtr(!d.External)
	browser.Pause(ctx, time.Second, 2500*time.Millisecond)
	return nil
}

func (glints) OpenApply(ctx context.Context, p *rod.Page, l Listing) ([]string, string, error) {
	if err := browser.Goto(p, l.URL); err != nil {
		return nil, "", err
	}
	browser.Pause(ctx, 2*time.Second, 4*time.Second)
	text := strings.ToLower(browser.PageText(p, 5000))
	if strings.Contains(text, "sudah dilamar") || strings.Contains(text, "sudah melamar") || strings.Contains(text, "already applied") {
		return nil, "", ErrAlreadyApplied
	}
	if browser.HasText(p, `lamar di situs|apply on (company )?website|situs perusahaan`) {
		return nil, l.URL, nil
	}
	if _, ok := browser.ClickText(p, `^(lamar cepat|lamar sekarang|lamar|quick apply|apply now|apply)$`); ok {
		browser.Pause(ctx, 2*time.Second, 4*time.Second)
		return []string{`[role="dialog"]`, `[class*="Modal" i]`}, "", nil
	}
	return nil, "", fmt.Errorf("couldn't find the Lamar / Apply button")
}

// ChallengeError means the portal showed a human check; the account is
// paused until the user resolves it in the worker's browser.
type ChallengeError struct {
	Portal string
	What   string
}

func (e *ChallengeError) Error() string {
	return fmt.Sprintf("%s showed a %s", PortalLabel[e.Portal], e.What)
}
