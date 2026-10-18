// Package analytics records site views and CV downloads with where and when
// they came from, and summarises them for the dashboard overview.
//
// Location comes from Cloudflare, which sits in front of the site: it adds
// CF-IPCountry to every request, and city/region once "Add visitor location
// headers" is enabled (Rules → Settings → Managed Transforms).
package analytics

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/text/language"
	"golang.org/x/text/language/display"
)

type Repo struct{ db *pgxpool.Pool }

func NewRepo(db *pgxpool.Pool) *Repo { return &Repo{db: db} }

type Visit struct {
	Kind        string
	IP          string
	CountryCode string
	Region      string
	City        string
	Referrer    string
	Device      string
	Browser     string
	OS          string
	UserAgent   string
	Bot         bool
	Owner       bool
}

// FromRequest reads a visit off the request. trustProxy says whether the
// Cloudflare and proxy headers can be believed.
func FromRequest(r *http.Request, kind string, trustProxy, owner bool) Visit {
	ua := r.UserAgent()
	v := Visit{Kind: kind, UserAgent: clip(ua, 300), Owner: owner, Bot: IsBot(ua)}
	v.Device, v.Browser, v.OS = ParseUA(ua)
	if v.Bot {
		v.Device = "bot"
	}
	// The server's CloudflareIP middleware has already put the visitor's
	// address in RemoteAddr.
	ip := remoteIP(r)
	if trustProxy {
		cc := strings.ToUpper(strings.TrimSpace(r.Header.Get("CF-IPCountry")))
		if len(cc) == 2 && cc != "XX" && cc != "T1" {
			v.CountryCode = cc
		}
		v.Region = clip(header(r, "CF-Region"), 80)
		v.City = clip(header(r, "CF-IPCity"), 80)
	}
	v.IP = MaskIP(ip)
	if ref, err := url.Parse(r.Referer()); err == nil && ref.Host != "" && !strings.EqualFold(ref.Hostname(), hostOnly(r.Host)) {
		v.Referrer = strings.TrimPrefix(strings.ToLower(ref.Hostname()), "www.")
	}
	return v
}

// header decodes Cloudflare's location values, which arrive percent-encoded
// for non-ASCII names.
func header(r *http.Request, name string) string {
	s := strings.TrimSpace(r.Header.Get(name))
	if d, err := url.QueryUnescape(s); err == nil {
		return d
	}
	return s
}

// Record stores a visit in the background so it never slows the response.
func (r *Repo) Record(ctx context.Context, v Visit) {
	go func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_, err := r.db.Exec(ctx, `INSERT INTO visits (kind, ip, country_code, region, city, referrer, device, browser,
			os, user_agent, is_bot, is_owner) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
			v.Kind, v.IP, v.CountryCode, v.Region, v.City, v.Referrer, v.Device, v.Browser, v.OS, v.UserAgent, v.Bot, v.Owner)
		if err != nil {
			slog.Warn("record visit", "err", err)
		}
	}()
}

// MaskIP keeps the network part: 203.0.113.0 for IPv4, the first 48 bits for IPv6.
func MaskIP(s string) string {
	ip := net.ParseIP(strings.TrimSpace(s))
	if ip == nil {
		return ""
	}
	if v4 := ip.To4(); v4 != nil {
		return v4.Mask(net.CIDRMask(24, 32)).String()
	}
	return ip.Mask(net.CIDRMask(48, 128)).String()
}

func remoteIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func hostOnly(h string) string {
	if host, _, err := net.SplitHostPort(h); err == nil {
		return host
	}
	return h
}

var botUA = regexp.MustCompile(`(?i)bot\b|bot/|crawl|spider|slurp|scrap|fetch|preview|monitor|uptime|lighthouse|headless|phantom|` +
	`curl|wget|python|go-http|java/|okhttp|axios|node-fetch|libwww|httpclient|facebookexternalhit|whatsapp|telegram|` +
	`discord|slack|skype|linkedinbot|embedly|gptbot|chatgpt|claudebot|claude-web|anthropic|perplexity|bytespider|ccbot|` +
	`ahrefs|semrush|mj12|dotbot|petalbot|yandex|baidu|bingpreview|google-|applebot|duckduck`)

// IsBot recognises crawlers, link previews and scripts by user agent. An
// empty user agent is a script, not a person.
func IsBot(ua string) bool {
	return strings.TrimSpace(ua) == "" || botUA.MatchString(ua)
}

// ParseUA is a small user-agent reader: device class, browser and OS family.
func ParseUA(ua string) (device, browser, os string) {
	l := strings.ToLower(ua)
	switch {
	case strings.Contains(l, "ipad") || strings.Contains(l, "tablet") || (strings.Contains(l, "android") && !strings.Contains(l, "mobile")):
		device = "tablet"
	case strings.Contains(l, "mobi") || strings.Contains(l, "iphone") || strings.Contains(l, "android"):
		device = "mobile"
	default:
		device = "desktop"
	}
	switch {
	case strings.Contains(l, "edg/"):
		browser = "Edge"
	case strings.Contains(l, "opr/") || strings.Contains(l, "opera"):
		browser = "Opera"
	case strings.Contains(l, "samsungbrowser"):
		browser = "Samsung Internet"
	case strings.Contains(l, "firefox/") || strings.Contains(l, "fxios"):
		browser = "Firefox"
	case strings.Contains(l, "chrome/") || strings.Contains(l, "crios"):
		browser = "Chrome"
	case strings.Contains(l, "safari/"):
		browser = "Safari"
	default:
		browser = "Other"
	}
	switch {
	case strings.Contains(l, "iphone") || strings.Contains(l, "ipad") || strings.Contains(l, "ios"):
		os = "iOS"
	case strings.Contains(l, "android"):
		os = "Android"
	case strings.Contains(l, "windows"):
		os = "Windows"
	case strings.Contains(l, "mac os") || strings.Contains(l, "macintosh"):
		os = "macOS"
	case strings.Contains(l, "linux"):
		os = "Linux"
	default:
		os = "Other"
	}
	return
}

// CountryName turns "ID" into "Indonesia".
func CountryName(code string) string {
	if code == "" {
		return "Unknown"
	}
	r, err := language.ParseRegion(code)
	if err != nil {
		return code
	}
	if n := display.English.Regions().Name(r); n != "" {
		return n
	}
	return code
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// ── report ───────────────────────────────────────────────────────────────────

type Day struct {
	Day   string `json:"day"`
	Views int    `json:"views"`
	CV    int    `json:"cv"`
}

type Hour struct {
	Hour  int `json:"hour"`
	Views int `json:"views"`
	CV    int `json:"cv"`
}

type Place struct {
	Code  string `json:"code"`
	Name  string `json:"name"`
	Views int    `json:"views"`
	CV    int    `json:"cv"`
}

type Referrer struct {
	Host  string `json:"host"`
	Views int    `json:"views"`
	CV    int    `json:"cv"`
}

type Event struct {
	At       time.Time `json:"at"`
	Kind     string    `json:"kind"`
	IP       string    `json:"ip"`
	Country  string    `json:"country"`
	Code     string    `json:"countryCode"`
	Region   string    `json:"region"`
	City     string    `json:"city"`
	Referrer string    `json:"referrer"`
	Device   string    `json:"device"`
	Browser  string    `json:"browser"`
	OS       string    `json:"os"`
	Bot      bool      `json:"bot"`
}

type Report struct {
	Days      int        `json:"days"`
	Timezone  string     `json:"timezone"`
	Views     int        `json:"views"`
	CV        int        `json:"cv"`
	Visitors  int        `json:"visitors"`
	Bots      int        `json:"bots"`
	HasCity   bool       `json:"hasCity"`
	Since     *time.Time `json:"since"`
	Daily     []Day      `json:"daily"`
	Hours     []Hour     `json:"hours"`
	Countries []Place    `json:"countries"`
	Cities    []Place    `json:"cities"`
	Referrers []Referrer `json:"referrers"`
	RecentCV  []Event    `json:"recentCv"`
}

// Report summarises the last `days` days in tz. People only: bots and the
// owner's own visits are counted separately (Bots) or not at all.
func (r *Repo) Report(ctx context.Context, days int, tz *time.Location) (Report, error) {
	rep := Report{Days: days, Timezone: tz.String(), Daily: []Day{}, Hours: make([]Hour, 24)}
	now := time.Now().In(tz)
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, tz).AddDate(0, 0, -(days - 1))
	const people = `at >= $1 AND NOT is_bot AND NOT is_owner`

	err := r.db.QueryRow(ctx, `SELECT
			count(*) FILTER (WHERE kind='view' AND NOT is_bot AND NOT is_owner),
			count(*) FILTER (WHERE kind='cv' AND NOT is_bot AND NOT is_owner),
			count(DISTINCT ip || ':' || user_agent) FILTER (WHERE NOT is_bot AND NOT is_owner AND ip <> ''),
			count(*) FILTER (WHERE is_bot),
			bool_or(city <> '') IS TRUE
		FROM visits WHERE at >= $1`, start).Scan(&rep.Views, &rep.CV, &rep.Visitors, &rep.Bots, &rep.HasCity)
	if err != nil {
		return rep, err
	}
	_ = r.db.QueryRow(ctx, `SELECT min(at) FROM visits`).Scan(&rep.Since)

	// Daily, with empty days filled in.
	byDay := map[string]*Day{}
	for d := start; !d.After(now); d = d.AddDate(0, 0, 1) {
		rep.Daily = append(rep.Daily, Day{Day: d.Format("2006-01-02")})
	}
	for i := range rep.Daily {
		byDay[rep.Daily[i].Day] = &rep.Daily[i]
	}
	rows, err := r.db.Query(ctx, `SELECT to_char(at AT TIME ZONE $2, 'YYYY-MM-DD'), extract(hour FROM at AT TIME ZONE $2)::int,
			count(*) FILTER (WHERE kind='view'), count(*) FILTER (WHERE kind='cv')
		FROM visits WHERE `+people+` GROUP BY 1, 2`, start, tz.String())
	if err != nil {
		return rep, err
	}
	for rows.Next() {
		var day string
		var hour, views, cv int
		if err := rows.Scan(&day, &hour, &views, &cv); err != nil {
			return rep, err
		}
		if d := byDay[day]; d != nil {
			d.Views += views
			d.CV += cv
		}
		if hour >= 0 && hour < 24 {
			rep.Hours[hour].Views += views
			rep.Hours[hour].CV += cv
		}
	}
	rows.Close()
	for h := range rep.Hours {
		rep.Hours[h].Hour = h
	}

	if rep.Countries, err = r.places(ctx, `country_code`, start); err != nil {
		return rep, err
	}
	for i := range rep.Countries {
		rep.Countries[i].Name = CountryName(rep.Countries[i].Code)
	}
	if rep.Cities, err = r.places(ctx, `city`, start); err != nil {
		return rep, err
	}

	rows, err = r.db.Query(ctx, `SELECT referrer, count(*) FILTER (WHERE kind='view'), count(*) FILTER (WHERE kind='cv')
		FROM visits WHERE `+people+` AND referrer <> '' GROUP BY 1 ORDER BY 2 DESC, 3 DESC LIMIT 8`, start)
	if err != nil {
		return rep, err
	}
	rep.Referrers, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (Referrer, error) {
		var x Referrer
		return x, row.Scan(&x.Host, &x.Views, &x.CV)
	})
	if err != nil {
		return rep, err
	}

	// Recent CV downloads, bots included (flagged) so nothing is hidden.
	rows, err = r.db.Query(ctx, `SELECT at, kind, ip, country_code, region, city, referrer, device, browser, os, is_bot
		FROM visits WHERE kind='cv' AND at >= $1 AND NOT is_owner ORDER BY at DESC LIMIT 30`, start)
	if err != nil {
		return rep, err
	}
	rep.RecentCV, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (Event, error) {
		var e Event
		err := row.Scan(&e.At, &e.Kind, &e.IP, &e.Code, &e.Region, &e.City, &e.Referrer, &e.Device, &e.Browser, &e.OS, &e.Bot)
		e.Country = CountryName(e.Code)
		return e, err
	})
	return rep, err
}

// places groups people's visits by country_code or city, top 10.
func (r *Repo) places(ctx context.Context, col string, start time.Time) ([]Place, error) {
	q := `SELECT country_code, '' AS label, count(*) FILTER (WHERE kind='view') AS v, count(*) FILTER (WHERE kind='cv') AS c
		FROM visits WHERE at >= $1 AND NOT is_bot AND NOT is_owner GROUP BY 1 ORDER BY count(*) DESC, 1 LIMIT 10`
	if col == "city" {
		q = `SELECT country_code, city || CASE WHEN region <> '' AND region <> city THEN ', ' || region ELSE '' END AS label,
			count(*) FILTER (WHERE kind='view') AS v, count(*) FILTER (WHERE kind='cv') AS c
			FROM visits WHERE at >= $1 AND NOT is_bot AND NOT is_owner AND city <> '' GROUP BY 1, 2 ORDER BY count(*) DESC, 2 LIMIT 10`
	}
	rows, err := r.db.Query(ctx, q, start)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Place, error) {
		var p Place
		var label string
		err := row.Scan(&p.Code, &label, &p.Views, &p.CV)
		if col == "city" {
			p.Name = label
		}
		return p, err
	})
}
