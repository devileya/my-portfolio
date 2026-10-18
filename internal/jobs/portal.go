package jobs

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-rod/rod"

	"github.com/devileya/my-portfolio/internal/jobs/browser"
)

// Portal is one job site. Search and Details may use plain HTTP or the
// browser; Apply always uses the browser with the account's session.
type Portal interface {
	Name() string
	// CookieDomains are the domains whose cookies make up a login session.
	CookieDomains() []string
	LoginURL() string
	// LoggedIn inspects the session; it may navigate p.
	LoggedIn(ctx context.Context, p *rod.Page) (bool, error)
	Search(ctx context.Context, env *Env, s Search, keyword, location string, limit int) ([]Listing, error)
	// Details fills Description (and EasyApply when the search didn't say).
	Details(ctx context.Context, env *Env, l *Listing) error
	// OpenApply opens the listing and starts its application. It returns the
	// selectors of the form container (empty = whole page), or an external
	// URL when the portal sends applicants to the company's own site.
	OpenApply(ctx context.Context, p *rod.Page, l Listing) (roots []string, external string, err error)
}

// Cookie is a portal session cookie, stored sealed in job_accounts.
type Cookie = browser.Cookie

// Env is what adapters may use.
type Env struct {
	HTTP     *http.Client
	Browser  *browser.Browser
	Headless bool
}

func (e *Env) Page(ctx context.Context) (*rod.Page, error) { return e.Browser.Page(ctx, e.Headless) }

var portals = map[string]Portal{
	"linkedin": linkedin{},
	"indeed":   indeed{},
	"glints":   glints{},
	"kalibrr":  kalibrr{},
}

func PortalByName(name string) (Portal, bool) {
	p, ok := portals[name]
	return p, ok
}

// ErrAlreadyApplied is returned by OpenApply when the portal says so.
var ErrAlreadyApplied = fmt.Errorf("the portal says you already applied")

const userAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/141.0.0.0 Safari/537.36"

func httpGet(ctx context.Context, c *http.Client, u string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept-Language", "en-US,en;q=0.9,id;q=0.8")
	res, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 16<<20))
	if err != nil {
		return nil, err
	}
	if res.StatusCode == http.StatusTooManyRequests {
		return nil, fmt.Errorf("%s is rate-limiting requests; the next run will retry", hostOf(u))
	}
	if res.StatusCode >= 300 {
		return nil, fmt.Errorf("%s answered HTTP %d", hostOf(u), res.StatusCode)
	}
	return body, nil
}

func hostOf(u string) string {
	if p, err := url.Parse(u); err == nil {
		return p.Host
	}
	return u
}

// loggedOutHeader reports visible "Sign in"/"Masuk" style controls near the
// top of the page: portals without a reliable session cookie are judged by it.
func loggedOutHeader(p *rod.Page) bool {
	var out bool
	_ = browser.Eval(p, `() => {
		const rx = /^(sign in|log ?in|masuk|login|daftar|sign up|register|join now|gabung)$/i;
		return [...document.querySelectorAll('a, button')].some(el => {
			const r = el.getBoundingClientRect();
			return r.width > 0 && r.height > 0 && r.top < 160 && rx.test((el.innerText || '').replace(/\s+/g, ' ').trim());
		});
	}`, &out)
	return out
}

func hasCookie(ctx context.Context, p *rod.Page, name string) bool {
	cookies, err := p.Context(ctx).Browser().GetCookies()
	if err != nil {
		return false
	}
	for _, c := range cookies {
		if c.Name == name && c.Value != "" && (c.Expires == 0 || time.Unix(int64(c.Expires), 0).After(time.Now())) {
			return true
		}
	}
	return false
}

func slugify(s string) string {
	s = normWords(strings.ToLower(s))
	return strings.ReplaceAll(s, " ", "-")
}

func boolPtr(b bool) *bool { return &b }
