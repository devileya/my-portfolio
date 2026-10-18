package jobs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/devileya/my-portfolio/internal/jobs/browser"
	"github.com/devileya/my-portfolio/internal/telegram"
)

// Worker runs browser work one item at a time: tasks first (logins, session
// checks, searches), then at most one application per loop, paced per portal.
type Worker struct {
	svc      *Service
	name     string
	browser  *browser.Browser
	env      *Env
	tmpDir   string
	injected map[string]bool      // portals whose DB cookies are in this Chrome
	nextOK   map[string]time.Time // earliest next application per portal
	lastUse  time.Time
}

type WorkerOptions struct {
	Name       string
	BrowserDir string
	BrowserBin string
	Headless   bool
}

func NewWorker(svc *Service, o WorkerOptions) *Worker {
	b := browser.New(browser.Options{Dir: o.BrowserDir, Bin: o.BrowserBin, Headless: o.Headless})
	return &Worker{
		svc: svc, name: o.Name, browser: b,
		env:      &Env{HTTP: &http.Client{Timeout: 30 * time.Second}, Browser: b, Headless: o.Headless},
		tmpDir:   filepath.Join(os.TempDir(), "portfolio-jobs"),
		injected: map[string]bool{},
		nextOK:   map[string]time.Time{},
	}
}

func (w *Worker) Run(ctx context.Context) {
	slog.Info("job worker started", "name", w.name)
	defer w.browser.Close()
	_ = w.svc.Repo.RecoverStale(ctx)
	lastRecover := time.Now()
	for ctx.Err() == nil {
		busy := w.tick(ctx)
		if time.Since(lastRecover) > 10*time.Minute {
			_ = w.svc.Repo.RecoverStale(ctx)
			lastRecover = time.Now()
		}
		if !w.lastUse.IsZero() && time.Since(w.lastUse) > 5*time.Minute {
			w.browser.Close() // free memory between bursts of work
			w.injected = map[string]bool{}
			w.lastUse = time.Time{}
		}
		if busy {
			continue
		}
		select {
		case <-ctx.Done():
		case <-time.After(15 * time.Second):
		}
	}
	slog.Info("job worker stopped")
}

// tick does one unit of work and reports whether it did any.
func (w *Worker) tick(ctx context.Context) bool {
	set, err := w.svc.Repo.Settings(ctx)
	if err != nil {
		slog.Warn("job worker settings", "err", err)
		return false
	}
	if set.Paused {
		_ = w.svc.Repo.Heartbeat(ctx, w.name, "paused")
		return false
	}
	t, err := w.svc.Repo.ClaimTask(ctx, w.name)
	if err == nil {
		w.lastUse = time.Now()
		w.runTask(ctx, t)
		return true
	}
	if !errors.Is(err, ErrNotFound) {
		slog.Warn("claim task", "err", err)
		return false
	}
	if w.applyNext(ctx, set) {
		w.lastUse = time.Now()
		return true
	}
	_ = w.svc.Repo.Heartbeat(ctx, w.name, "idle")
	return false
}

// ── tasks ────────────────────────────────────────────────────────────────────

type taskLog struct{ b strings.Builder }

func (l *taskLog) printf(format string, args ...any) {
	fmt.Fprintf(&l.b, time.Now().Format("15:04:05")+" "+format+"\n", args...)
}

func (w *Worker) runTask(ctx context.Context, t Task) {
	ctx, cancel := context.WithTimeout(ctx, 40*time.Minute)
	defer cancel()
	log := &taskLog{}
	var result any
	var err error
	switch t.Kind {
	case "search":
		_ = w.svc.Repo.Heartbeat(ctx, w.name, fmt.Sprintf("searching %s for %q", PortalLabel[t.Portal], t.SearchName))
		result, err = w.search(ctx, t, log)
	case "login":
		_ = w.svc.Repo.Heartbeat(ctx, w.name, "waiting for "+PortalLabel[t.Portal]+" sign-in")
		err = w.login(ctx, t.Portal, log)
	case "check":
		_ = w.svc.Repo.Heartbeat(ctx, w.name, "checking "+PortalLabel[t.Portal])
		err = w.check(ctx, t.Portal, log)
	default:
		err = fmt.Errorf("unknown task kind %q", t.Kind)
	}
	status, errText := "done", ""
	if err != nil {
		status, errText = "failed", err.Error()
		log.printf("error: %v", err)
		slog.Warn("job task failed", "id", t.ID, "kind", t.Kind, "portal", t.Portal, "err", err)
	}
	if ferr := w.svc.Repo.FinishTask(context.WithoutCancel(ctx), t.ID, status, result, log.b.String(), errText); ferr != nil {
		slog.Error("finish task", "id", t.ID, "err", ferr)
	}
}

func (w *Worker) search(ctx context.Context, t Task, log *taskLog) (TaskResult, error) {
	var res TaskResult
	if t.SearchID == nil {
		return res, errors.New("the search was deleted")
	}
	srch, err := w.svc.Repo.Search(ctx, *t.SearchID)
	if err != nil {
		return res, err
	}
	portal, ok := PortalByName(t.Portal)
	if !ok {
		return res, fmt.Errorf("unknown portal %q", t.Portal)
	}
	acct, err := w.svc.Repo.Account(ctx, t.Portal)
	if err != nil {
		return res, err
	}
	if !acct.Enabled {
		log.printf("%s is turned off in Accounts; skipped", PortalLabel[t.Portal])
		return res, nil
	}
	if err := w.prepare(ctx, portal); err != nil {
		return res, err
	}

	keywords := srch.Keywords
	if len(keywords) == 0 {
		keywords = []string{srch.Name}
	}
	locations := srch.Locations
	if len(locations) == 0 {
		locations = []string{""}
	}
	perQuery := max(5, srch.MaxResults/max(1, len(keywords)*len(locations)))

	var found []Listing
	for _, kw := range keywords {
		for _, loc := range locations {
			list, err := portal.Search(ctx, w.env, srch, kw, loc, perQuery)
			log.printf("search %q in %q: %d results", kw, orLabel(loc, "anywhere"), len(list))
			found = append(found, list...)
			if err != nil {
				if w.challenge(ctx, err) {
					return res, err
				}
				log.printf("search error: %v", err)
				if len(found) == 0 {
					return res, err
				}
			}
		}
	}
	res.Found = len(found)

	// Store new listings, then filter, enrich and score them.
	var fresh []Listing
	for _, l := range found {
		l.SearchID = &srch.ID
		isNew, err := w.svc.Repo.InsertListing(ctx, &l)
		if err != nil {
			return res, err
		}
		if isNew {
			fresh = append(fresh, l)
		}
	}
	res.New = len(fresh)
	log.printf("%d found, %d new", res.Found, res.New)
	if len(fresh) == 0 {
		return res, w.svc.Repo.SetNextRun(ctx, srch.ID, srch.NextRunAt, true)
	}

	app, err := w.svc.Applicant(ctx)
	if err != nil {
		return res, err
	}
	client, err := w.svc.AI(ctx, "score")
	if err != nil {
		return res, err
	}

	var queued []Listing
	for _, l := range fresh {
		if ctx.Err() != nil {
			break
		}
		if reason, err := w.svc.filterReason(ctx, srch, l); err != nil {
			return res, err
		} else if reason != "" {
			res.Filtered++
			_ = w.svc.Repo.SetListingStatus(ctx, l.ID, StatusFiltered, reason)
			continue
		}
		if err := portal.Details(ctx, w.env, &l); err != nil {
			if w.challenge(ctx, err) {
				return res, err
			}
			log.printf("details for %q: %v", l.Title, err)
		} else {
			_ = w.svc.Repo.UpdateListingDetails(ctx, l)
		}
		// Details may reveal it's an external apply or another mode.
		if reason, err := w.svc.filterReason(ctx, srch, l); err == nil && reason != "" {
			res.Filtered++
			_ = w.svc.Repo.SetListingStatus(ctx, l.ID, StatusFiltered, reason)
			continue
		}

		sc, err := w.svc.score(ctx, client, app, srch, l)
		if err != nil {
			log.printf("score %q: %v", l.Title, err)
			_ = w.svc.Repo.SetListingStatus(ctx, l.ID, StatusNew, "Not scored yet: "+Clip(err.Error(), 200))
			if errors.Is(err, context.DeadlineExceeded) {
				break
			}
			continue
		}
		res.Scored++
		status := StatusFiltered
		switch {
		case srch.AutoApply && sc.Score >= srch.AutoMinScore:
			status = StatusApproved
			res.Approved++
		case sc.Score >= srch.MinScore:
			status = StatusQueued
			res.Queued++
		}
		d := ScoreDetails{Verdict: sc.Verdict, Matched: sc.Matched, Missing: sc.Missing, Concerns: sc.Concerns}
		if err := w.svc.Repo.SetScore(ctx, l.ID, sc.Score, sc.Reason, d, status); err != nil {
			return res, err
		}
		l.Score, l.ScoreReason, l.Status, l.ScoreDetails = &sc.Score, sc.Reason, status, d
		if status != StatusFiltered {
			queued = append(queued, l)
		}
		log.printf("%3d  %s @ %s → %s", sc.Score, l.Title, l.Company, status)
	}
	log.printf("filtered %d, queued %d, auto-approved %d", res.Filtered, res.Queued, res.Approved)
	w.svc.notifyMatches(ctx, srch, queued)
	_ = w.svc.Repo.SetNextRun(ctx, srch.ID, srch.NextRunAt, true)
	w.saveSession(ctx, portal)
	return res, nil
}

func (w *Worker) login(ctx context.Context, portalName string, log *taskLog) error {
	portal, ok := PortalByName(portalName)
	if !ok {
		return fmt.Errorf("unknown portal %q", portalName)
	}
	// Logins need a visible window; reuse the same profile.
	if err := w.prepare(ctx, portal); err != nil {
		return err
	}
	p, err := w.browser.Page(ctx, false)
	if err != nil {
		return err
	}
	defer p.Close()
	if err := browser.Goto(p, portal.LoginURL()); err != nil {
		return err
	}
	label := PortalLabel[portalName]
	log.printf("opened %s sign-in in the worker's Chrome window", label)
	w.svc.Bot().Notify(ctx, fmt.Sprintf("🔐 Sign in to <b>%s</b> in the Chrome window on <b>%s</b>. I'll wait up to 10 minutes.",
		label, telegram.Escape(w.name)))
	deadline := time.Now().Add(10 * time.Minute)
	for time.Now().Before(deadline) && ctx.Err() == nil {
		browser.Pause(ctx, 6*time.Second, 6*time.Second)
		u := strings.ToLower(browser.URL(p))
		if strings.Contains(u, "login") || strings.Contains(u, "signin") || strings.Contains(u, "/auth") ||
			strings.Contains(u, "checkpoint") || strings.Contains(u, "challenge") {
			continue
		}
		// Off the sign-in pages: confirm in a second tab so the user's tab stays put.
		check, err := w.browser.Page(ctx, false)
		if err != nil {
			return err
		}
		ok, err := portal.LoggedIn(ctx, check)
		check.Close()
		if err == nil && ok {
			cookies, err := w.browser.Cookies(portal.CookieDomains()...)
			if err != nil {
				return err
			}
			if err := w.svc.Repo.SaveCookies(ctx, portalName, cookies); err != nil {
				return err
			}
			_ = w.svc.Repo.SetAccountStatus(ctx, portalName, "connected", "")
			w.injected[portalName] = true
			log.printf("signed in; saved %d cookies", len(cookies))
			w.svc.Bot().Notify(ctx, "✅ "+label+" is connected.")
			return nil
		}
	}
	_ = w.svc.Repo.SetAccountStatus(ctx, portalName, "disconnected", "Sign-in wasn't completed in time.")
	return errors.New("sign-in wasn't completed within 10 minutes")
}

func (w *Worker) check(ctx context.Context, portalName string, log *taskLog) error {
	portal, ok := PortalByName(portalName)
	if !ok {
		return fmt.Errorf("unknown portal %q", portalName)
	}
	if err := w.prepare(ctx, portal); err != nil {
		return err
	}
	p, err := w.env.Page(ctx)
	if err != nil {
		return err
	}
	defer p.Close()
	ok, err = portal.LoggedIn(ctx, p)
	if err != nil {
		if w.challenge(ctx, err) {
			return nil
		}
		_ = w.svc.Repo.SetAccountStatus(ctx, portalName, "error", Clip(err.Error(), 300))
		return err
	}
	if ok {
		log.printf("signed in")
		_ = w.svc.Repo.SetAccountStatus(ctx, portalName, "connected", "")
		w.saveSession(ctx, portal)
		return nil
	}
	log.printf("signed out")
	w.expired(ctx, portalName)
	return nil
}

// prepare starts Chrome and loads the portal's stored cookies once per launch.
func (w *Worker) prepare(ctx context.Context, portal Portal) error {
	if _, err := w.browser.Start(w.env.Headless); err != nil {
		return err
	}
	if w.injected[portal.Name()] {
		return nil
	}
	cookies, err := w.svc.Repo.Cookies(ctx, portal.Name())
	if err != nil {
		return err
	}
	if err := w.browser.SetCookies(cookies); err != nil {
		return fmt.Errorf("load %s cookies: %w", PortalLabel[portal.Name()], err)
	}
	w.injected[portal.Name()] = true
	return nil
}

// saveSession writes the browser's current cookies back, so refreshed
// session tokens survive a restart or a move to another machine.
func (w *Worker) saveSession(ctx context.Context, portal Portal) {
	acct, err := w.svc.Repo.Account(ctx, portal.Name())
	if err != nil || acct.Status != "connected" {
		return
	}
	cookies, err := w.browser.Cookies(portal.CookieDomains()...)
	if err == nil && len(cookies) > 0 {
		_ = w.svc.Repo.SaveCookies(ctx, portal.Name(), cookies)
	}
}

// challenge handles a ChallengeError: mark the account and tell the user.
func (w *Worker) challenge(ctx context.Context, err error) bool {
	var ch *ChallengeError
	if !errors.As(err, &ch) {
		return false
	}
	acct, _ := w.svc.Repo.Account(ctx, ch.Portal)
	_ = w.svc.Repo.SetAccountStatus(ctx, ch.Portal, "challenge", ch.Error())
	if acct.Status != "challenge" {
		w.svc.Bot().Notify(ctx, fmt.Sprintf("🛑 <b>%s</b> showed a %s. %s is paused until you clear it: open Job Hunter → Accounts → <i>Sign in</i>, which opens Chrome on %s.",
			PortalLabel[ch.Portal], telegram.Escape(ch.What), PortalLabel[ch.Portal], telegram.Escape(w.name)))
	}
	return true
}

func (w *Worker) expired(ctx context.Context, portal string) {
	acct, _ := w.svc.Repo.Account(ctx, portal)
	_ = w.svc.Repo.SetAccountStatus(ctx, portal, "expired", "Signed out; sign in again.")
	if acct.Status == "connected" {
		w.svc.Bot().Notify(ctx, fmt.Sprintf("⚠️ The <b>%s</b> session expired. Sign in again from Job Hunter → Accounts.", PortalLabel[portal]))
	}
}

// ── applying ─────────────────────────────────────────────────────────────────

// applyNext picks the best approved listing that its portal's pacing, caps
// and the application window allow, and applies to it.
func (w *Worker) applyNext(ctx context.Context, set Settings) bool {
	loc := set.Location()
	now := time.Now().In(loc)
	if h := now.Hour(); h < set.ApplyStartHour || h >= set.ApplyEndHour {
		return false
	}
	candidates, err := w.svc.Repo.ApprovedCandidates(ctx)
	if err != nil || len(candidates) == 0 {
		return false
	}
	accounts, err := w.svc.Repo.Accounts(ctx)
	if err != nil {
		return false
	}
	acct := map[string]Account{}
	for _, a := range accounts {
		acct[a.Portal] = a
	}
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	searchCaps := map[int64]int{}

	for _, l := range candidates {
		a := acct[l.Portal]
		if !a.Enabled || a.Status != "connected" {
			continue
		}
		if t, ok := w.nextOK[l.Portal]; ok && time.Now().Before(t) {
			continue
		}
		if last, err := w.svc.Repo.LastAttemptAt(ctx, l.Portal); err == nil && last != nil &&
			time.Since(*last) < time.Duration(set.MinGapMinutes)*time.Minute {
			continue
		}
		if cap := set.PortalCaps[l.Portal]; cap > 0 {
			if n, err := w.svc.Repo.AppliedSince(ctx, l.Portal, today); err != nil || n >= cap {
				continue
			}
		}
		if l.SearchID != nil {
			cap, seen := searchCaps[*l.SearchID]
			if !seen {
				if srch, err := w.svc.Repo.Search(ctx, *l.SearchID); err == nil {
					cap = srch.DailyCap
				}
				searchCaps[*l.SearchID] = cap
			}
			if cap > 0 {
				if n, err := w.svc.Repo.AppliedForSearchSince(ctx, *l.SearchID, today); err != nil || n >= cap {
					continue
				}
			}
		}
		if ok, err := w.svc.Repo.ClaimListing(ctx, l.ID); err != nil || !ok {
			continue
		}
		w.applyOne(ctx, set, l)
		gap := set.MinGapMinutes
		if set.MaxGapMinutes > gap {
			gap += rand.IntN(set.MaxGapMinutes - gap + 1)
		}
		w.nextOK[l.Portal] = time.Now().Add(time.Duration(gap) * time.Minute)
		return true
	}
	return false
}

func (w *Worker) applyOne(ctx context.Context, set Settings, l Listing) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Minute)
	defer cancel()
	label := PortalLabel[l.Portal]
	_ = w.svc.Repo.Heartbeat(ctx, w.name, fmt.Sprintf("applying: %s @ %s (%s)", l.Title, l.Company, label))
	appID, err := w.svc.Repo.StartApplication(ctx, l.ID)
	if err != nil {
		slog.Error("start application", "err", err)
		_ = w.svc.Repo.SetListingStatus(ctx, l.ID, StatusApproved, "")
		return
	}
	rec := Application{ID: appID, ListingID: l.ID}
	finish := func(res applyResult) {
		bg := context.WithoutCancel(ctx)
		rec.Status, rec.Answers, rec.PendingQuestions, rec.CoverLetter, rec.Steps = res.Status, res.Answers, res.Pending, res.Cover, res.Steps
		if res.Status != "submitted" && res.Status != "dry_run" && res.Status != "needs_input" && res.Status != "external" {
			rec.Error = res.Note
		}
		if res.Status == "challenge" {
			rec.Status = "failed"
		}
		if len(res.Shot) > 0 && w.svc.Store != nil {
			if obj, err := w.svc.Store.Put(bg, "jobs/screens", ".jpg", "image/jpeg", res.Shot); err == nil {
				rec.ScreenshotKey = obj.Key
			}
		}
		if err := w.svc.Repo.FinishApplication(bg, rec); err != nil {
			slog.Error("finish application", "err", err)
		}
		status := map[string]string{
			"submitted": StatusApplied, "dry_run": StatusDryRun, "needs_input": StatusNeedsInput,
			"external": StatusExternal, "challenge": StatusApproved, "failed": StatusFailed,
		}[res.Status]
		if res.Status == "failed" && l.Attempts+1 < 3 {
			status = StatusApproved // retried on a later loop
		}
		_ = w.svc.Repo.SetListingStatus(bg, l.ID, status, Clip(res.Note, 500))
		l.Status = status
		w.svc.notifyApplied(bg, l, rec, res)
	}

	portal, ok := PortalByName(l.Portal)
	if !ok {
		finish(applyResult{Status: "failed", Note: "unknown portal"})
		return
	}
	app, err := w.svc.Applicant(ctx)
	if err == nil {
		app.CVPath, app.CVName, err = w.svc.DownloadCV(ctx, w.tmpDir)
	}
	if err == nil {
		err = w.prepare(ctx, portal)
	}
	if err != nil {
		finish(applyResult{Status: "failed", Note: err.Error()})
		return
	}
	p, err := w.env.Page(ctx)
	if err != nil {
		finish(applyResult{Status: "failed", Note: err.Error()})
		return
	}
	before := pageIDs(p)
	defer func() {
		// Close the job tab and any tab the application flow opened.
		if pages, err := p.Browser().Pages(); err == nil {
			for _, pg := range pages {
				if !before[string(pg.TargetID)] {
					_ = pg.Close()
				}
			}
		}
		_ = p.Close()
	}()

	res := w.svc.apply(ctx, p, portal, l, app, set.DryRun)
	if res.Status == "challenge" {
		w.challenge(ctx, &ChallengeError{Portal: l.Portal, What: strings.TrimPrefix(res.Note, label+" showed a ")})
	}
	if res.Status == "failed" && strings.Contains(strings.ToLower(browser.URL(p)), "login") {
		w.expired(ctx, l.Portal)
		res.Status, res.Note = "challenge", "Signed out of "+label+"; sign in again."
	}
	finish(res)
	if res.Status == "submitted" {
		w.saveSession(ctx, portal)
	}
}
