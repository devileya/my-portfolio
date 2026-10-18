package jobs

import (
	"context"
	"fmt"
	"html"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/go-rod/rod"

	"github.com/devileya/my-portfolio/internal/jobs/browser"
)

// LinkedIn: search and details through the public (guest) job endpoints, so
// browsing never touches the account; applying uses Easy Apply only.
type linkedin struct{}

func (linkedin) Name() string            { return "linkedin" }
func (linkedin) CookieDomains() []string { return []string{"linkedin.com"} }
func (linkedin) LoginURL() string        { return "https://www.linkedin.com/login" }

func (linkedin) LoggedIn(ctx context.Context, p *rod.Page) (bool, error) {
	if err := browser.Goto(p, "https://www.linkedin.com/feed/"); err != nil {
		return false, err
	}
	u := browser.URL(p)
	if strings.Contains(u, "/login") || strings.Contains(u, "/authwall") || strings.Contains(u, "/signup") || strings.Contains(u, "/uas/") {
		return false, nil
	}
	return hasCookie(ctx, p, "li_at"), nil
}

var (
	liCard     = regexp.MustCompile(`(?s)data-entity-urn="urn:li:jobPosting:(\d+)".*?</li>`)
	liTitle    = regexp.MustCompile(`(?s)base-search-card__title">\s*(.*?)\s*</h3>`)
	liCompany  = regexp.MustCompile(`(?s)base-search-card__subtitle">.*?>\s*([^<]+?)\s*<`)
	liLocation = regexp.MustCompile(`(?s)job-search-card__location">\s*(.*?)\s*</span>`)
	liDate     = regexp.MustCompile(`datetime="(\d{4}-\d{2}-\d{2})"`)
	liSalary   = regexp.MustCompile(`(?s)job-search-card__salary-info">\s*(.*?)\s*</span>`)
	liDesc     = regexp.MustCompile(`(?s)show-more-less-html__markup[^>]*>(.*?)</div>`)
	liCriteria = regexp.MustCompile(`(?s)description__job-criteria-subheader">\s*(.*?)\s*</h3>.*?description__job-criteria-text[^>]*>\s*(.*?)\s*</span>`)
)

var liWorkMode = map[string]string{"onsite": "1", "remote": "2", "hybrid": "3"}
var liJobType = map[string]string{"full_time": "F", "contract": "C", "part_time": "P", "internship": "I", "freelance": "C"}
var liLevel = map[string]string{"internship": "1", "entry": "2", "associate": "3", "mid": "4", "senior": "4", "lead": "4", "director": "5"}

func (linkedin) Search(ctx context.Context, env *Env, s Search, keyword, location string, limit int) ([]Listing, error) {
	// One query per work mode so each result knows its mode.
	modes := s.WorkModes
	if len(modes) == 0 {
		modes = []string{""}
	}
	var out []Listing
	seen := map[string]bool{}
	for _, mode := range modes {
		q := url.Values{"keywords": {keyword}}
		if location != "" {
			q.Set("location", location)
		}
		if s.PostedWithinDays > 0 {
			q.Set("f_TPR", fmt.Sprintf("r%d", s.PostedWithinDays*86400))
		}
		if code := liWorkMode[mode]; code != "" {
			q.Set("f_WT", code)
		}
		q.Set("f_JT", joinCodes(s.JobTypes, liJobType))
		q.Set("f_E", joinCodes(s.Seniority, liLevel))
		if s.EasyApplyOnly {
			q.Set("f_AL", "true")
		}
		for k, v := range q {
			if len(v) == 1 && v[0] == "" {
				delete(q, k)
			}
		}
		for start := 0; len(out) < limit && start < 100; start += 10 {
			q.Set("start", fmt.Sprint(start))
			body, err := httpGet(ctx, env.HTTP, "https://www.linkedin.com/jobs-guest/jobs/api/seeMoreJobPostings/search?"+q.Encode())
			if err != nil {
				if len(out) > 0 {
					return out, nil
				}
				return nil, err
			}
			cards := liCard.FindAllStringSubmatch(string(body), -1)
			if len(cards) == 0 {
				break
			}
			for _, c := range cards {
				id := c[1]
				if seen[id] {
					continue
				}
				seen[id] = true
				l := Listing{
					Portal: "linkedin", ExternalID: id, WorkMode: mode,
					URL:      "https://www.linkedin.com/jobs/view/" + id + "/",
					Title:    cleanHTML(first(liTitle, c[0])),
					Company:  cleanHTML(first(liCompany, c[0])),
					Location: cleanHTML(first(liLocation, c[0])),
					Salary:   cleanHTML(first(liSalary, c[0])),
				}
				if d, err := time.Parse("2006-01-02", first(liDate, c[0])); err == nil {
					l.PostedAt = &d
				}
				if l.Title != "" {
					out = append(out, l)
				}
			}
			browser.Pause(ctx, 1200*time.Millisecond, 2500*time.Millisecond)
		}
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (linkedin) Details(ctx context.Context, env *Env, l *Listing) error {
	body, err := httpGet(ctx, env.HTTP, "https://www.linkedin.com/jobs-guest/jobs/api/jobPosting/"+l.ExternalID)
	if err != nil {
		return err
	}
	page := string(body)
	l.Description = HTMLText(first(liDesc, page))
	for _, m := range liCriteria.FindAllStringSubmatch(page, -1) {
		if strings.EqualFold(cleanHTML(m[1]), "Employment type") && l.JobType == "" {
			l.JobType = normJobType(cleanHTML(m[2]))
		}
	}
	switch {
	case strings.Contains(page, "apply-link-simple_onsite"):
		l.EasyApply = boolPtr(true)
	case strings.Contains(page, "apply-link-offsite"):
		l.EasyApply = boolPtr(false)
	}
	browser.Pause(ctx, 800*time.Millisecond, 1800*time.Millisecond)
	return nil
}

func (linkedin) OpenApply(ctx context.Context, p *rod.Page, l Listing) ([]string, string, error) {
	if err := browser.Goto(p, l.URL); err != nil {
		return nil, "", err
	}
	browser.Pause(ctx, 2*time.Second, 4*time.Second)
	text := strings.ToLower(browser.PageText(p, 6000))
	if strings.Contains(text, "applied ") && (strings.Contains(text, "application submitted") || strings.Contains(text, "see application")) {
		return nil, "", ErrAlreadyApplied
	}
	if _, ok := browser.ClickText(p, `^(easy apply|lamar mudah)$`); ok {
		browser.Pause(ctx, 1500*time.Millisecond, 3*time.Second)
		return []string{".jobs-easy-apply-modal", `[data-test-modal-id="easy-apply-modal"]`, ".artdeco-modal"}, "", nil
	}
	if browser.HasText(p, `^(apply|lamar)$`) {
		return nil, l.URL, nil
	}
	if strings.Contains(text, "no longer accepting applications") || strings.Contains(text, "tidak lagi menerima") {
		return nil, "", fmt.Errorf("the job is no longer accepting applications")
	}
	return nil, "", fmt.Errorf("couldn't find the Easy Apply button")
}

// ── shared helpers ───────────────────────────────────────────────────────────

func first(re *regexp.Regexp, s string) string {
	if m := re.FindStringSubmatch(s); len(m) > 1 {
		return m[1]
	}
	return ""
}

var tags = regexp.MustCompile(`<[^>]+>`)

func cleanHTML(s string) string {
	return strings.TrimSpace(spaces.ReplaceAllString(html.UnescapeString(tags.ReplaceAllString(s, " ")), " "))
}

func joinCodes(values []string, codes map[string]string) string {
	var out []string
	seen := map[string]bool{}
	for _, v := range values {
		if c := codes[v]; c != "" && !seen[c] {
			seen[c] = true
			out = append(out, c)
		}
	}
	return strings.Join(out, ",")
}

// normJobType maps portal wording to our job types.
func normJobType(s string) string {
	s = strings.ToLower(s)
	switch {
	case strings.Contains(s, "full"), strings.Contains(s, "penuh"), strings.Contains(s, "permanent"):
		return "full_time"
	case strings.Contains(s, "part"), strings.Contains(s, "paruh"):
		return "part_time"
	case strings.Contains(s, "intern"), strings.Contains(s, "magang"):
		return "internship"
	case strings.Contains(s, "freelance"), strings.Contains(s, "project"), strings.Contains(s, "lepas"):
		return "freelance"
	case strings.Contains(s, "contract"), strings.Contains(s, "kontrak"), strings.Contains(s, "temporary"):
		return "contract"
	}
	return ""
}
