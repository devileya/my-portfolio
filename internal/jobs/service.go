package jobs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/devileya/my-portfolio/internal/ai"
	"github.com/devileya/my-portfolio/internal/content"
	"github.com/devileya/my-portfolio/internal/storage"
	"github.com/devileya/my-portfolio/internal/telegram"
)

// Service holds what both the server (scheduler, Telegram, dashboard) and
// the worker (browser) need.
type Service struct {
	Repo      *Repo
	Content   *content.Repo
	Store     *storage.Store
	PublicURL string
	AdminURL  string // absolute dashboard URL for links in notifications, or ""

	// The Telegram bot is configured in the dashboard and swapped in place
	// when its settings change; see RunTelegram.
	bot      atomic.Pointer[telegram.Bot]
	botKey   atomic.Value // token|chat|poll the current bot was built from
	reloadCh chan struct{}
	once     sync.Once
}

// Bot is the current Telegram bot, or nil (every method on nil is a no-op).
func (s *Service) Bot() *telegram.Bot { return s.bot.Load() }

// ReloadTelegram asks RunTelegram to pick up new settings now.
func (s *Service) ReloadTelegram() {
	select {
	case s.reload() <- struct{}{}:
	default:
	}
}

func (s *Service) reload() chan struct{} {
	s.once.Do(func() { s.reloadCh = make(chan struct{}, 1) })
	return s.reloadCh
}

// loadBot rebuilds the bot from the database when its settings changed. It
// reports whether polling should run.
func (s *Service) loadBot(ctx context.Context) (changed, poll bool, err error) {
	set, err := s.Repo.Settings(ctx)
	if err != nil {
		return false, false, err
	}
	token, chat, err := s.Repo.TelegramToken(ctx)
	if err != nil {
		return false, false, err
	}
	key := fmt.Sprintf("%s|%d|%v", token, chat, set.Telegram.Poll)
	if old, _ := s.botKey.Load().(string); old == key {
		return false, set.Telegram.Poll, nil
	}
	s.botKey.Store(key)
	s.bot.Store(telegram.New(token, chat))
	return true, set.Telegram.Poll, nil
}

// RunTelegram keeps the bot in step with the dashboard settings. With poll
// set (the server), it also long-polls for button presses and replies,
// restarting the poller whenever the bot changes.
func (s *Service) RunTelegram(ctx context.Context, poll bool) {
	reload := s.reload()
	var stop context.CancelFunc
	defer func() {
		if stop != nil {
			stop()
		}
	}()
	for {
		changed, wantPoll, err := s.loadBot(ctx)
		if err != nil && ctx.Err() == nil {
			slog.Warn("telegram settings", "err", err)
		}
		if changed && stop != nil {
			stop()
			stop = nil
		}
		if poll && stop == nil && wantPoll && s.Bot() != nil {
			var pctx context.Context
			pctx, stop = context.WithCancel(ctx)
			go s.Bot().Poll(pctx, s.HandleTelegram)
		}
		if poll && !wantPoll && stop != nil {
			stop()
			stop = nil
		}
		select {
		case <-ctx.Done():
			return
		case <-reload:
		case <-time.After(30 * time.Second):
		}
	}
}

var ErrNoProvider = errors.New("choose an AI provider in Job Hunter → Settings first")

// AI returns the client for the scoring or the form-filling role. The apply
// role falls back to the scoring provider when it isn't set.
func (s *Service) AI(ctx context.Context, role string) (ai.Client, error) {
	set, err := s.Repo.Settings(ctx)
	if err != nil {
		return nil, err
	}
	id := set.ScoreProviderID
	if role == "apply" && set.ApplyProviderID != nil {
		id = set.ApplyProviderID
	}
	if id == nil {
		return nil, ErrNoProvider
	}
	p, err := s.Repo.AIProvider(ctx, *id)
	if err != nil {
		return nil, err
	}
	return ai.New(p)
}

// Applicant is the candidate as the models see it: the portfolio profile,
// timeline and skills, plus the form facts from Job Hunter settings.
type Applicant struct {
	Name     string
	Email    string
	Text     string // profile rendered for prompts
	CVPath   string // local file, set by the worker
	CVName   string
	Facts    map[string]string
	Settings Settings
}

func (s *Service) Applicant(ctx context.Context) (Applicant, error) {
	p, err := s.Content.Profile(ctx)
	if err != nil {
		return Applicant{}, err
	}
	exps, err := s.Content.Experiences(ctx, true)
	if err != nil {
		return Applicant{}, err
	}
	skills, err := s.Content.Skills(ctx)
	if err != nil {
		return Applicant{}, err
	}
	set, err := s.Repo.Settings(ctx)
	if err != nil {
		return Applicant{}, err
	}

	facts := map[string]string{"fullName": p.Name, "email": p.Email, "location": p.Location}
	if p.YearsExperience > 0 {
		facts["yearsOfExperience"] = fmt.Sprint(p.YearsExperience)
	}
	if s.PublicURL != "" {
		facts["portfolioUrl"] = s.PublicURL
	}
	for _, soc := range p.Socials {
		switch l := strings.ToLower(soc.Label); {
		case strings.Contains(l, "linkedin"):
			facts["linkedinUrl"] = soc.URL
		case strings.Contains(l, "github"):
			facts["githubUrl"] = soc.URL
		}
	}
	if first, last, ok := strings.Cut(p.Name, " "); ok {
		facts["firstName"], facts["lastName"] = first, last
	}
	for k, v := range set.Applicant {
		if strings.TrimSpace(v) != "" {
			facts[k] = v
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Name: %s\nRole: %s\nLocation: %s\nYears of professional experience: %d\n", p.Name, p.Role, p.Location, p.YearsExperience)
	if p.Summary != "" {
		fmt.Fprintf(&b, "Summary: %s\n", p.Summary)
	}
	if p.Bio != "" {
		fmt.Fprintf(&b, "Bio: %s\n", Clip(p.Bio, 1500))
	}
	if len(p.FocusAreas) > 0 {
		fmt.Fprintf(&b, "Focus areas: %s\n", strings.Join(p.FocusAreas, ", "))
	}
	if len(skills) > 0 {
		byLevel := map[string][]string{}
		for _, sk := range skills {
			byLevel[sk.Level] = append(byLevel[sk.Level], sk.Name)
		}
		b.WriteString("Skills:\n")
		for _, lvl := range []string{"Expert", "Advanced", "Intermediate", "Basic"} {
			if len(byLevel[lvl]) > 0 {
				fmt.Fprintf(&b, "  %s: %s\n", lvl, strings.Join(byLevel[lvl], ", "))
			}
		}
	}
	b.WriteString("Experience and education (newest first):\n")
	for _, e := range exps {
		end := e.PeriodEnd
		if end == "" && e.Kind == "work" {
			end = "present"
		}
		fmt.Fprintf(&b, "- [%s] %s, %s (%s – %s)\n", e.Kind, e.Title, e.Organization, e.PeriodStart, end)
		if e.Summary != "" {
			fmt.Fprintf(&b, "    %s\n", Clip(e.Summary, 400))
		}
		for _, h := range e.Highlights {
			fmt.Fprintf(&b, "    • %s\n", Clip(h, 250))
		}
		if len(e.Tech) > 0 {
			fmt.Fprintf(&b, "    Tech: %s\n", strings.Join(e.Tech, ", "))
		}
	}
	b.WriteString("Facts for application forms:\n")
	for _, k := range sortedKeys(facts) {
		fmt.Fprintf(&b, "  %s: %s\n", k, facts[k])
	}

	return Applicant{Name: p.Name, Email: p.Email, Text: b.String(), Facts: facts, Settings: set}, nil
}

// DownloadCV copies the portfolio's CV from storage to dir and returns its path.
func (s *Service) DownloadCV(ctx context.Context, dir string) (string, string, error) {
	key, name, err := s.Content.CVKey(ctx)
	if err != nil {
		return "", "", err
	}
	if key == "" {
		return "", "", errors.New("upload a CV in the dashboard first: forms ask for one")
	}
	obj, err := s.Store.Get(ctx, key)
	if err != nil {
		return "", "", fmt.Errorf("download CV: %w", err)
	}
	defer obj.Close()
	if name == "" {
		name = "CV.pdf"
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", "", err
	}
	path := filepath.Join(dir, filepath.Base(name))
	f, err := os.Create(path)
	if err != nil {
		return "", "", err
	}
	defer f.Close()
	if _, err := f.ReadFrom(obj); err != nil {
		return "", "", err
	}
	return path, name, nil
}

func (s *Service) link(path string) string {
	if s.AdminURL == "" {
		return ""
	}
	return s.AdminURL + path
}
