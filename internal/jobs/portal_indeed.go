package jobs

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/go-rod/rod"

	"github.com/devileya/my-portfolio/internal/jobs/browser"
)

// Indeed sits behind Cloudflare: everything goes through the browser, and a
// challenge pauses the portal until the user clears it once in the worker's
// Chrome window (the profile keeps the clearance afterwards).
type indeed struct{}

// indeedHost is the country site searched; applications go through Indeed
// Apply on smartapply.indeed.com regardless.
const indeedHost = "https://id.indeed.com"

func (indeed) Name() string            { return "indeed" }
func (indeed) CookieDomains() []string { return []string{"indeed.com"} }
func (indeed) LoginURL() string        { return "https://secure.indeed.com/auth" }

func (indeed) LoggedIn(ctx context.Context, p *rod.Page) (bool, error) {
	if err := browser.Goto(p, indeedHost+"/"); err != nil {
		return false, err
	}
	if ch := browser.Challenge(p); ch != "" {
		return false, &ChallengeError{Portal: "indeed", What: ch}
	}
	return hasCookie(ctx, p, "SHOE") || !loggedOutHeader(p), nil
}

type indeedCard struct {
	JobKey       string `json:"jobkey"`
	Title        string `json:"displayTitle"`
	Company      string `json:"company"`
	Location     string `json:"formattedLocation"`
	IndeedApply  bool   `json:"indeedApplyEnabled"`
	PubDate      int64  `json:"pubDate"`
	Snippet      string `json:"snippet"`
	SalaryText   string `json:"salaryText"`
	RemoteType   string `json:"remoteType"`
	JobTypesText string `json:"jobTypes"`
}

func (indeed) Search(ctx context.Context, env *Env, s Search, keyword, location string, limit int) ([]Listing, error) {
	q := url.Values{"q": {keyword}}
	if location != "" && !strings.EqualFold(location, "remote") {
		q.Set("l", location)
	} else if len(s.WorkModes) == 1 && s.WorkModes[0] == "remote" {
		q.Set("l", "remote")
	}
	if s.PostedWithinDays > 0 {
		q.Set("fromage", fmt.Sprint(min(s.PostedWithinDays, 14)))
	}
	p, err := env.Page(ctx)
	if err != nil {
		return nil, err
	}
	defer p.Close()

	var out []Listing
	seen := map[string]bool{}
	for start := 0; len(out) < limit && start < 50; start += 10 {
		q.Set("start", fmt.Sprint(start))
		if err := browser.Goto(p, indeedHost+"/jobs?"+q.Encode()); err != nil {
			return out, err
		}
		if ch := browser.Challenge(p); ch != "" {
			return out, &ChallengeError{Portal: "indeed", What: ch}
		}
		var cards []indeedCard
		if err := browser.Eval(p, `() => {
			const d = window.mosaic && window.mosaic.providerData && window.mosaic.providerData['mosaic-provider-jobcards'];
			const r = (d && d.metaData && d.metaData.mosaicProviderJobCardsModel && d.metaData.mosaicProviderJobCardsModel.results) || [];
			return r.map(j => ({
				jobkey: j.jobkey, displayTitle: j.displayTitle || j.title || '', company: j.company || '',
				formattedLocation: j.formattedLocation || '', indeedApplyEnabled: !!j.indeedApplyEnabled,
				pubDate: j.pubDate || 0, snippet: (j.snippet || '').replace(/<[^>]+>/g, ' '),
				salaryText: (j.salarySnippet && j.salarySnippet.text) || '',
				remoteType: (j.remoteWorkModel && j.remoteWorkModel.type) || '',
				jobTypes: (j.jobTypes || []).join(', '),
			}));
		}`, &cards); err != nil {
			return out, fmt.Errorf("indeed: the results page changed: %w", err)
		}
		if len(cards) == 0 {
			break
		}
		for _, c := range cards {
			if c.JobKey == "" || seen[c.JobKey] {
				continue
			}
			seen[c.JobKey] = true
			l := Listing{
				Portal: "indeed", ExternalID: c.JobKey, Title: c.Title, Company: c.Company, Location: c.Location,
				URL: indeedHost + "/viewjob?jk=" + c.JobKey, Salary: c.SalaryText, JobType: normJobType(c.JobTypesText),
				Description: HTMLText(c.Snippet), EasyApply: boolPtr(c.IndeedApply),
			}
			switch rt := strings.ToUpper(c.RemoteType); {
			case strings.Contains(rt, "HYBRID"):
				l.WorkMode = "hybrid"
			case strings.Contains(rt, "REMOTE"):
				l.WorkMode = "remote"
			case strings.Contains(strings.ToLower(c.Location), "remote"), strings.Contains(strings.ToLower(c.Location), "jarak jauh"):
				l.WorkMode = "remote"
			}
			if c.PubDate > 0 {
				t := time.UnixMilli(c.PubDate)
				l.PostedAt = &t
			}
			out = append(out, l)
		}
		browser.Pause(ctx, 3*time.Second, 7*time.Second)
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (indeed) Details(ctx context.Context, env *Env, l *Listing) error {
	p, err := env.Page(ctx)
	if err != nil {
		return err
	}
	defer p.Close()
	if err := browser.Goto(p, l.URL); err != nil {
		return err
	}
	if ch := browser.Challenge(p); ch != "" {
		return &ChallengeError{Portal: "indeed", What: ch}
	}
	var d struct {
		Text string `json:"text"`
		Easy bool   `json:"easy"`
	}
	_ = browser.Eval(p, `() => {
		const desc = document.getElementById('jobDescriptionText');
		const easy = !!document.querySelector('#indeedApplyButton, [data-testid="indeedApplyButton"], .ia-IndeedApplyButton');
		return { text: desc ? desc.innerText.slice(0, 12000) : '', easy };
	}`, &d)
	if d.Text != "" {
		l.Description = d.Text
	}
	l.EasyApply = boolPtr(d.Easy)
	browser.Pause(ctx, 2*time.Second, 5*time.Second)
	return nil
}

func (indeed) OpenApply(ctx context.Context, p *rod.Page, l Listing) ([]string, string, error) {
	if err := browser.Goto(p, l.URL); err != nil {
		return nil, "", err
	}
	if ch := browser.Challenge(p); ch != "" {
		return nil, "", &ChallengeError{Portal: "indeed", What: ch}
	}
	browser.Pause(ctx, 2*time.Second, 4*time.Second)
	text := strings.ToLower(browser.PageText(p, 5000))
	if strings.Contains(text, "you applied") || strings.Contains(text, "applied on") || strings.Contains(text, "anda sudah melamar") {
		return nil, "", ErrAlreadyApplied
	}
	if browser.HasText(p, `apply on company site|lamar di situs perusahaan|apply on company website`) {
		return nil, l.URL, nil
	}
	if _, ok := browser.ClickText(p, `^(apply now|easily apply|lamar sekarang|lamar dengan mudah)$`); ok {
		browser.Pause(ctx, 3*time.Second, 5*time.Second)
		return nil, "", nil
	}
	return nil, "", fmt.Errorf("couldn't find the Apply now button")
}
