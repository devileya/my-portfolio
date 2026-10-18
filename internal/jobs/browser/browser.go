// Package browser drives a real Chrome for the job portals: a persistent
// profile (so sessions survive restarts), cookie import/export, and the
// form-reading helpers the application flow is built on.
//
// It never solves CAPTCHAs or other human checks. When a portal shows one,
// Challenge reports it and the caller pauses and asks the user to step in.
package browser

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
	"github.com/go-rod/stealth"
)

type Options struct {
	Dir      string // persistent user-data dir
	Bin      string // Chrome binary; empty = look it up
	Headless bool
}

// Cookie is the portable cookie shape stored (sealed) in the database. It
// also accepts the JSON exported by browser extensions such as Cookie-Editor.
type Cookie struct {
	Name           string  `json:"name"`
	Value          string  `json:"value"`
	Domain         string  `json:"domain"`
	Path           string  `json:"path"`
	Expires        float64 `json:"expires,omitempty"`
	ExpirationDate float64 `json:"expirationDate,omitempty"`
	HTTPOnly       bool    `json:"httpOnly"`
	Secure         bool    `json:"secure"`
	SameSite       string  `json:"sameSite,omitempty"`
}

type Browser struct {
	opts     Options
	mu       sync.Mutex
	b        *rod.Browser
	launcher *launcher.Launcher
	headless bool
}

func New(opts Options) *Browser { return &Browser{opts: opts} }

// Start launches Chrome if it isn't running (or relaunches it when the
// requested visibility differs). The profile dir allows one Chrome at a time.
func (b *Browser) Start(headless bool) (*rod.Browser, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.b != nil && b.headless == headless {
		return b.b, nil
	}
	b.closeLocked()
	if err := os.MkdirAll(b.opts.Dir, 0o700); err != nil {
		return nil, fmt.Errorf("browser profile dir: %w", err)
	}
	bin := b.opts.Bin
	if bin == "" {
		var ok bool
		if bin, ok = launcher.LookPath(); !ok {
			return nil, errors.New("Chrome was not found. Install Google Chrome or set JOBS_BROWSER_BIN")
		}
	}
	l := launcher.New().Bin(bin).Headless(headless).UserDataDir(b.opts.Dir).
		Set("disable-blink-features", "AutomationControlled").
		Set("window-size", "1366,900").
		Set("lang", "en-US,en,id").
		Delete("enable-automation")
	url, err := l.Launch()
	if err != nil {
		return nil, fmt.Errorf("launch Chrome: %w", err)
	}
	rb := rod.New().ControlURL(url)
	if err := rb.Connect(); err != nil {
		l.Kill()
		return nil, fmt.Errorf("connect to Chrome: %w", err)
	}
	b.b, b.launcher, b.headless = rb, l, headless
	return rb, nil
}

func (b *Browser) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.closeLocked()
}

func (b *Browser) closeLocked() {
	if b.b != nil {
		_ = b.b.Close()
		b.b = nil
	}
	if b.launcher != nil {
		b.launcher.Kill()
		b.launcher.Cleanup()
		b.launcher = nil
	}
}

// Page opens a new tab with common automation fingerprints masked.
func (b *Browser) Page(ctx context.Context, headless bool) (*rod.Page, error) {
	rb, err := b.Start(headless)
	if err != nil {
		return nil, err
	}
	p, err := stealth.Page(rb)
	if err != nil {
		// A dead connection: relaunch once.
		b.Close()
		if rb, err = b.Start(headless); err != nil {
			return nil, err
		}
		if p, err = stealth.Page(rb); err != nil {
			return nil, err
		}
	}
	_ = proto.EmulationSetDeviceMetricsOverride{Width: 1366, Height: 900, DeviceScaleFactor: 1}.Call(p)
	return p.Context(ctx), nil
}

// ── cookies ──────────────────────────────────────────────────────────────────

func (b *Browser) SetCookies(cookies []Cookie) error {
	if len(cookies) == 0 {
		return nil
	}
	rb, err := b.Start(b.currentHeadless())
	if err != nil {
		return err
	}
	params := make([]*proto.NetworkCookieParam, 0, len(cookies))
	for _, c := range cookies {
		if c.Name == "" || c.Domain == "" {
			continue
		}
		exp := c.Expires
		if exp == 0 {
			exp = c.ExpirationDate
		}
		p := &proto.NetworkCookieParam{
			Name: c.Name, Value: c.Value, Domain: c.Domain, Path: c.Path,
			Secure: c.Secure, HTTPOnly: c.HTTPOnly, SameSite: sameSite(c.SameSite),
		}
		if p.Path == "" {
			p.Path = "/"
		}
		if exp > 0 {
			p.Expires = proto.TimeSinceEpoch(exp)
		}
		params = append(params, p)
	}
	return rb.SetCookies(params)
}

// Cookies exports the cookies whose domain ends with one of suffixes.
func (b *Browser) Cookies(suffixes ...string) ([]Cookie, error) {
	rb, err := b.Start(b.currentHeadless())
	if err != nil {
		return nil, err
	}
	all, err := rb.GetCookies()
	if err != nil {
		return nil, err
	}
	var out []Cookie
	for _, c := range all {
		d := strings.TrimPrefix(c.Domain, ".")
		for _, s := range suffixes {
			if d == s || strings.HasSuffix(d, "."+s) {
				out = append(out, Cookie{
					Name: c.Name, Value: c.Value, Domain: c.Domain, Path: c.Path, Expires: float64(c.Expires),
					HTTPOnly: c.HTTPOnly, Secure: c.Secure, SameSite: string(c.SameSite),
				})
				break
			}
		}
	}
	return out, nil
}

func (b *Browser) currentHeadless() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.b != nil {
		return b.headless
	}
	return b.opts.Headless
}

func sameSite(s string) proto.NetworkCookieSameSite {
	switch strings.ToLower(s) {
	case "strict":
		return proto.NetworkCookieSameSiteStrict
	case "lax":
		return proto.NetworkCookieSameSiteLax
	case "none", "no_restriction":
		return proto.NetworkCookieSameSiteNone
	}
	return ""
}

// ── page helpers ─────────────────────────────────────────────────────────────

// Goto navigates and waits for the page to settle.
func Goto(p *rod.Page, url string) error {
	if err := p.Timeout(45 * time.Second).Navigate(url); err != nil {
		return fmt.Errorf("open %s: %w", url, err)
	}
	_ = p.Timeout(30 * time.Second).WaitLoad()
	_ = p.Timeout(8*time.Second).WaitDOMStable(700*time.Millisecond, 0.02)
	return nil
}

// Pause sleeps a human-looking random time between min and max.
func Pause(ctx context.Context, min, max time.Duration) {
	d := min
	if max > min {
		d += time.Duration(rand.Int64N(int64(max - min)))
	}
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
}

func URL(p *rod.Page) string {
	info, err := p.Info()
	if err != nil {
		return ""
	}
	return info.URL
}

// Eval runs a JS function and decodes its JSON-able return value into out.
func Eval(p *rod.Page, js string, out any, args ...any) error {
	res, err := p.Timeout(30*time.Second).Eval(js, args...)
	if err != nil {
		return err
	}
	if out == nil {
		return nil
	}
	return res.Value.Unmarshal(out)
}

func Screenshot(p *rod.Page) ([]byte, error) {
	q := 70
	return p.Timeout(20*time.Second).Screenshot(false, &proto.PageCaptureScreenshot{
		Format: proto.PageCaptureScreenshotFormatJpeg, Quality: &q,
	})
}

// Challenge describes a CAPTCHA or human check on the page, or "".
func Challenge(p *rod.Page) string {
	var out string
	_ = Eval(p, `() => {
		const t = (document.title || '').toLowerCase();
		const body = (document.body ? document.body.innerText : '').slice(0, 4000).toLowerCase();
		const url = location.href.toLowerCase();
		if (t.includes('just a moment') || t.includes('security check') || t.includes('attention required')) return 'Cloudflare security check';
		if (url.includes('/checkpoint/') || url.includes('/challenge')) return 'Security checkpoint';
		const frames = [...document.querySelectorAll('iframe')].map(f => (f.src || '').toLowerCase());
		if (frames.some(s => s.includes('recaptcha') || s.includes('hcaptcha') || s.includes('turnstile') || s.includes('arkoselabs') || s.includes('funcaptcha'))) {
			const visible = [...document.querySelectorAll('iframe')].some(f => { const r = f.getBoundingClientRect(); return r.width > 60 && r.height > 60; });
			if (visible) return 'CAPTCHA';
		}
		if (body.includes('verify you are human') || body.includes("let's do a quick security check") || body.includes('additional verification required')) return 'Human verification';
		return '';
	}`, &out)
	return out
}
