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

// Kalibrr: search through the job board's JSON API (public), apply in the
// browser.
type kalibrr struct{}

func (kalibrr) Name() string            { return "kalibrr" }
func (kalibrr) CookieDomains() []string { return []string{"kalibrr.com"} }
func (kalibrr) LoginURL() string        { return "https://www.kalibrr.com/login" }

func (kalibrr) LoggedIn(ctx context.Context, p *rod.Page) (bool, error) {
	if err := browser.Goto(p, "https://www.kalibrr.com/home"); err != nil {
		return false, err
	}
	if strings.Contains(browser.URL(p), "/login") {
		return false, nil
	}
	return !loggedOutHeader(p), nil
}

type kalibrrJob struct {
	ID             int64   `json:"id"`
	Name           string  `json:"name"`
	Slug           string  `json:"slug"`
	Description    string  `json:"description"`
	Qualifications string  `json:"qualifications"`
	Tenure         string  `json:"tenure"`
	IsWFH          bool    `json:"is_work_from_home"`
	IsHybrid       bool    `json:"is_hybrid"`
	BaseSalary     float64 `json:"base_salary"`
	MaxSalary      float64 `json:"maximum_salary"`
	Currency       string  `json:"salary_currency"`
	Interval       string  `json:"salary_interval"`
	Activation     string  `json:"activation_date"`
	ApplyRedirect  *string `json:"apply_redirect_url"`
	CompanyName    string  `json:"company_name"`
	Company        struct {
		Code string `json:"code"`
		Name string `json:"name"`
	} `json:"company"`
	Location struct {
		Address struct {
			City    string `json:"city"`
			Region  string `json:"region"`
			Country string `json:"country"`
		} `json:"address_components"`
	} `json:"google_location"`
}

func (kalibrr) Search(ctx context.Context, env *Env, s Search, keyword, location string, limit int) ([]Listing, error) {
	q := url.Values{"limit": {"30"}, "text": {keyword}}
	if location != "" && !strings.EqualFold(location, "remote") {
		q.Set("location", location)
	}
	if len(s.WorkModes) == 1 && s.WorkModes[0] == "remote" {
		q.Set("is_work_from_home", "true")
	}
	var out []Listing
	for offset := 0; len(out) < limit && offset < 150; offset += 30 {
		q.Set("offset", fmt.Sprint(offset))
		body, err := httpGet(ctx, env.HTTP, "https://www.kalibrr.com/kjs/job_board/search?"+q.Encode())
		if err != nil {
			if len(out) > 0 {
				return out, nil
			}
			return nil, err
		}
		var res struct {
			Jobs []kalibrrJob `json:"jobs"`
		}
		if err := json.Unmarshal(body, &res); err != nil {
			return nil, fmt.Errorf("kalibrr: unexpected search response")
		}
		for _, j := range res.Jobs {
			// The API can't filter hybrid or age, so drop mismatches here
			// instead of spending the run's result slots on them.
			l := j.listing()
			if len(s.WorkModes) > 0 && !containsFold(s.WorkModes, l.WorkMode) {
				continue
			}
			if s.PostedWithinDays > 0 && l.PostedAt != nil && time.Since(*l.PostedAt) > time.Duration(s.PostedWithinDays+1)*24*time.Hour {
				continue
			}
			out = append(out, l)
		}
		if len(res.Jobs) < 30 {
			break
		}
		browser.Pause(ctx, time.Second, 2*time.Second)
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (j kalibrrJob) listing() Listing {
	company := j.Company.Name
	if company == "" {
		company = j.CompanyName
	}
	l := Listing{
		Portal: "kalibrr", ExternalID: fmt.Sprint(j.ID), Title: j.Name, Company: company,
		URL:      fmt.Sprintf("https://www.kalibrr.com/c/%s/jobs/%d/%s", j.Company.Code, j.ID, j.Slug),
		Location: joinNonEmpty(", ", j.Location.Address.City, j.Location.Address.Region, j.Location.Address.Country),
		JobType:  normJobType(j.Tenure),
		WorkMode: "onsite",
	}
	switch {
	case j.IsWFH:
		l.WorkMode = "remote"
	case j.IsHybrid:
		l.WorkMode = "hybrid"
	}
	if j.BaseSalary > 0 || j.MaxSalary > 0 {
		l.Salary = strings.TrimSpace(fmt.Sprintf("%s %s – %s %s", j.Currency, money(j.BaseSalary), money(j.MaxSalary), j.Interval))
		lo, hi := int64(j.BaseSalary), int64(j.MaxSalary)
		if lo <= 0 {
			lo = hi
		}
		if hi <= 0 {
			hi = lo
		}
		l.SalaryMin, l.SalaryMax, l.SalaryCurrency, l.SalaryPeriod = &lo, &hi, j.Currency, normPeriod(j.Interval)
	}
	if t, err := time.Parse(time.RFC3339Nano, j.Activation); err == nil {
		l.PostedAt = &t
	}
	l.EasyApply = boolPtr(j.ApplyRedirect == nil || *j.ApplyRedirect == "")
	l.Description = strings.TrimSpace(HTMLText(j.Description) + "\n\nQualifications:\n" + HTMLText(j.Qualifications))
	return l
}

// Details: the search response already carries the description.
func (kalibrr) Details(context.Context, *Env, *Listing) error { return nil }

func (kalibrr) OpenApply(ctx context.Context, p *rod.Page, l Listing) ([]string, string, error) {
	if err := browser.Goto(p, l.URL); err != nil {
		return nil, "", err
	}
	browser.Pause(ctx, 2*time.Second, 4*time.Second)
	text := strings.ToLower(browser.PageText(p, 5000))
	if strings.Contains(text, "you have already applied") || strings.Contains(text, "already applied") || strings.Contains(text, "sudah melamar") {
		return nil, "", ErrAlreadyApplied
	}
	if _, ok := browser.ClickText(p, `^(apply now|apply for this job|apply|lamar sekarang|lamar)$`); ok {
		browser.Pause(ctx, 2*time.Second, 4*time.Second)
		return []string{`[role="dialog"]`, `[class*="modal" i]`}, "", nil
	}
	if browser.HasText(p, `apply on company|company website|situs perusahaan`) {
		return nil, l.URL, nil
	}
	return nil, "", fmt.Errorf("couldn't find the Apply button")
}

func money(v float64) string {
	if v <= 0 {
		return "?"
	}
	if v >= 1_000_000 {
		return fmt.Sprintf("%.1fM", v/1_000_000)
	}
	if v >= 1000 {
		return fmt.Sprintf("%.0fK", v/1000)
	}
	return fmt.Sprintf("%.0f", v)
}

func joinNonEmpty(sep string, parts ...string) string {
	var out []string
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" && !containsFold(out, p) {
			out = append(out, p)
		}
	}
	return strings.Join(out, sep)
}
