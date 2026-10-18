package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/devileya/my-portfolio/internal/ai"
	"github.com/devileya/my-portfolio/internal/jobs"
	"github.com/devileya/my-portfolio/internal/secrets"
	"github.com/devileya/my-portfolio/internal/storage"
	"github.com/devileya/my-portfolio/internal/telegram"
)

func (s *Server) jobRoutes(r chi.Router) {
	r.Get("/overview", s.jobsOverview)
	r.Get("/settings", s.jobsSettings)
	r.Put("/settings", s.putJobsSettings)
	r.Post("/pause", s.jobsPause)
	r.Put("/telegram/token", s.putTelegramToken)
	r.Get("/telegram/chats", s.telegramChats)
	r.Put("/telegram/chat", s.putTelegramChat)
	r.Delete("/telegram", s.deleteTelegram)
	r.Post("/telegram/test", s.jobsTelegramTest)
	r.Get("/meta", s.jobsMeta)

	r.Get("/providers", s.jobsProviders)
	r.Post("/providers", s.saveJobsProvider(false))
	r.Put("/providers/{id}", s.saveJobsProvider(true))
	r.Delete("/providers/{id}", s.deleteJobsProvider)
	r.Post("/providers/{id}/test", s.testJobsProvider)
	r.Get("/providers/{id}/models", s.jobsProviderModels)

	r.Get("/accounts", s.jobsAccounts)
	r.Put("/accounts/{portal}", s.putJobsAccount)
	r.Post("/accounts/{portal}/cookies", s.importJobsCookies)
	r.Delete("/accounts/{portal}/cookies", s.clearJobsCookies)
	r.Post("/accounts/{portal}/login", s.jobsAccountTask("login"))
	r.Post("/accounts/{portal}/check", s.jobsAccountTask("check"))

	r.Get("/searches", s.jobsSearches)
	r.Post("/searches", s.saveJobsSearch(false))
	r.Put("/searches/{id}", s.saveJobsSearch(true))
	r.Delete("/searches/{id}", s.deleteJobsSearch)
	r.Post("/searches/{id}/run", s.runJobsSearch)

	r.Get("/listings", s.jobsListings)
	r.Get("/listings/{id}", s.jobsListing)
	r.Post("/listings/{id}/review", s.reviewJobsListing)
	r.Post("/listings/review", s.bulkReviewJobs)
	r.Post("/listings/{id}/answers", s.answerJobsListing)
	r.Post("/listings/{id}/applied", s.markJobsApplied)

	r.Get("/applications", s.jobsApplications)

	r.Get("/answers", s.jobsAnswers)
	r.Post("/answers", s.saveJobsAnswer(false))
	r.Put("/answers/{id}", s.saveJobsAnswer(true))
	r.Delete("/answers/{id}", s.deleteJobsAnswer)

	r.Get("/tasks", s.jobsTasks)
	r.Delete("/tasks/{id}", s.cancelJobsTask)

	r.Get("/files/*", s.jobsFile)
}

// jobsFail maps repository errors to responses.
func jobsFail(w http.ResponseWriter, err error, what string) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, jobs.ErrNotFound):
		writeError(w, http.StatusNotFound, "That "+what+" no longer exists.")
	case errors.Is(err, secrets.ErrNoKey):
		writeError(w, http.StatusServiceUnavailable, "Set JOBS_SECRET_KEY in the server's .env (openssl rand -hex 32) to store keys and sessions.")
	case errors.Is(err, jobs.ErrNoProvider):
		writeError(w, http.StatusUnprocessableEntity, err.Error())
	default:
		slog.Error("jobs", "what", what, "err", err)
		writeError(w, http.StatusInternalServerError, "Something went wrong with the "+what+". Check the server log.")
	}
	return true
}

// ── overview & settings ──────────────────────────────────────────────────────

func (s *Server) jobsOverview(w http.ResponseWriter, r *http.Request) {
	o, err := s.jobs.Repo.Overview(r.Context())
	if jobsFail(w, err, "overview") {
		return
	}
	o.TelegramReady = s.jobs.Bot() != nil
	o.SecretsReady = s.jobs.Repo.SecretsReady()
	o.WorkerMode = s.cfg.JobsWorker
	writeJSON(w, http.StatusOK, o)
}

func (s *Server) jobsMeta(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"portals": jobs.Portals, "portalLabels": jobs.PortalLabel, "workModes": jobs.WorkModes,
		"jobTypes": jobs.JobTypes, "seniority": jobs.Seniority, "applicantFields": jobs.ApplicantFields,
	})
}

func (s *Server) jobsSettings(w http.ResponseWriter, r *http.Request) {
	set, err := s.jobs.Repo.Settings(r.Context())
	if jobsFail(w, err, "settings") {
		return
	}
	writeJSON(w, http.StatusOK, set)
}

func (s *Server) putJobsSettings(w http.ResponseWriter, r *http.Request) {
	var in jobs.Settings
	if !readJSON(w, r, &in) {
		return
	}
	if _, err := time.LoadLocation(in.Timezone); err != nil || in.Timezone == "" {
		writeError(w, http.StatusUnprocessableEntity, "Use an IANA time zone such as Asia/Jakarta.")
		return
	}
	if in.ApplyStartHour < 0 || in.ApplyEndHour > 24 || in.ApplyStartHour >= in.ApplyEndHour {
		writeError(w, http.StatusUnprocessableEntity, "The application window must start before it ends (hours 0–24).")
		return
	}
	if in.MinGapMinutes < 1 || in.MaxGapMinutes < in.MinGapMinutes {
		writeError(w, http.StatusUnprocessableEntity, "Use a minimum gap of at least 1 minute, and a maximum no lower than the minimum.")
		return
	}
	if in.DigestHour < 0 || in.DigestHour > 23 {
		writeError(w, http.StatusUnprocessableEntity, "The digest hour must be 0–23.")
		return
	}
	for p, n := range in.PortalCaps {
		if _, ok := jobs.PortalByName(p); !ok || n < 0 || n > 200 {
			writeError(w, http.StatusUnprocessableEntity, "Daily caps must be between 0 and 200 per portal.")
			return
		}
	}
	if err := s.jobs.Repo.SaveSettings(r.Context(), in); jobsFail(w, err, "settings") {
		return
	}
	s.jobsSettings(w, r)
}

func (s *Server) jobsPause(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Paused bool `json:"paused"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	if err := s.jobs.Repo.SetPaused(r.Context(), in.Paused); jobsFail(w, err, "settings") {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── Telegram ─────────────────────────────────────────────────────────────────

func (s *Server) putTelegramToken(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Token string `json:"token"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	in.Token = strings.TrimSpace(in.Token)
	if !strings.Contains(in.Token, ":") {
		writeError(w, http.StatusUnprocessableEntity, "Paste the token @BotFather gave you; it looks like 123456789:AA…")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	name, err := telegram.Verify(ctx, in.Token)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "Telegram rejected that token: "+strings.TrimPrefix(err.Error(), "telegram getMe: "))
		return
	}
	if jobsFail(w, s.jobs.Repo.SaveTelegramToken(r.Context(), in.Token, name), "Telegram settings") {
		return
	}
	s.jobs.ReloadTelegram()
	s.jobsSettings(w, r)
}

// telegramChats lists who messaged the bot, so the dashboard can offer the
// chat to send to without anyone looking up a numeric id.
func (s *Server) telegramChats(w http.ResponseWriter, r *http.Request) {
	token, _, err := s.jobs.Repo.TelegramToken(r.Context())
	if jobsFail(w, err, "Telegram settings") {
		return
	}
	if token == "" {
		writeError(w, http.StatusUnprocessableEntity, "Save the bot token first.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	chats, err := telegram.RecentChats(ctx, token)
	if err != nil {
		msg := err.Error()
		if strings.Contains(msg, "terminated by other getUpdates") || strings.Contains(msg, "Conflict") {
			msg = "Something else is reading this bot's messages right now. If you just pressed Change, wait a few seconds and try again; otherwise another app uses this bot, so create a separate one for Job Hunter."
		}
		writeError(w, http.StatusBadGateway, msg)
		return
	}
	if chats == nil {
		chats = []telegram.Chat{}
	}
	writeJSON(w, http.StatusOK, chats)
}

func (s *Server) putTelegramChat(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ChatID   int64  `json:"chatId"`
		ChatName string `json:"chatName"`
		Poll     bool   `json:"poll"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	if jobsFail(w, s.jobs.Repo.SaveTelegramChat(r.Context(), in.ChatID, strings.TrimSpace(in.ChatName), in.Poll), "Telegram settings") {
		return
	}
	s.jobs.ReloadTelegram()
	s.jobsSettings(w, r)
}

func (s *Server) deleteTelegram(w http.ResponseWriter, r *http.Request) {
	if jobsFail(w, s.jobs.Repo.ClearTelegram(r.Context()), "Telegram settings") {
		return
	}
	s.jobs.ReloadTelegram()
	s.jobsSettings(w, r)
}

func (s *Server) jobsTelegramTest(w http.ResponseWriter, r *http.Request) {
	token, chat, err := s.jobs.Repo.TelegramToken(r.Context())
	if jobsFail(w, err, "Telegram settings") {
		return
	}
	bot := telegram.New(token, chat)
	if bot == nil {
		writeError(w, http.StatusUnprocessableEntity, "Save a bot token and choose a chat first.")
		return
	}
	if _, err := bot.Send(r.Context(), "👋 Job Hunter can reach you here. Try /status."); err != nil {
		writeError(w, http.StatusBadGateway, "Telegram refused the message: "+err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── AI providers ─────────────────────────────────────────────────────────────

func (s *Server) jobsProviders(w http.ResponseWriter, r *http.Request) {
	list, err := s.jobs.Repo.Providers(r.Context())
	if jobsFail(w, err, "AI providers") {
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) saveJobsProvider(update bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in jobs.Provider
		if !readJSON(w, r, &in) {
			return
		}
		in.ID = 0
		if update {
			id, ok := idParam(w, r)
			if !ok {
				return
			}
			in.ID = id
		}
		in.Label, in.Model, in.BaseURL = strings.TrimSpace(in.Label), strings.TrimSpace(in.Model), strings.TrimSpace(in.BaseURL)
		switch {
		case in.Label == "":
			writeError(w, http.StatusUnprocessableEntity, "Give the provider a name.")
			return
		case in.Kind != "anthropic" && in.Kind != "openai":
			writeError(w, http.StatusUnprocessableEntity, "Choose Anthropic or an OpenAI-compatible API.")
			return
		case in.Model == "":
			writeError(w, http.StatusUnprocessableEntity, "Enter the model ID.")
			return
		case !update && strings.TrimSpace(in.APIKey) == "":
			writeError(w, http.StatusUnprocessableEntity, "Paste the API key.")
			return
		}
		if in.BaseURL != "" {
			if u, err := url.Parse(in.BaseURL); err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
				writeError(w, http.StatusUnprocessableEntity, "The base URL must be an http(s) URL, e.g. https://api.vikey.ai/v1.")
				return
			}
		}
		out, err := s.jobs.Repo.SaveProvider(r.Context(), in)
		if jobsFail(w, err, "AI provider") {
			return
		}
		// The first provider becomes the default for both roles.
		if set, err := s.jobs.Repo.Settings(r.Context()); err == nil && set.ScoreProviderID == nil {
			set.ScoreProviderID = &out.ID
			_ = s.jobs.Repo.SaveSettings(r.Context(), set)
		}
		status := http.StatusOK
		if !update {
			status = http.StatusCreated
		}
		writeJSON(w, status, out)
	}
}

func (s *Server) deleteJobsProvider(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	if jobsFail(w, s.jobs.Repo.DeleteProvider(r.Context(), id), "AI provider") {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) providerClient(w http.ResponseWriter, r *http.Request) (ai.Client, bool) {
	id, ok := idParam(w, r)
	if !ok {
		return nil, false
	}
	p, err := s.jobs.Repo.AIProvider(r.Context(), id)
	if jobsFail(w, err, "AI provider") {
		return nil, false
	}
	c, err := ai.New(p)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return nil, false
	}
	return c, true
}

func (s *Server) testJobsProvider(w http.ResponseWriter, r *http.Request) {
	c, ok := s.providerClient(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	start := time.Now()
	var out struct {
		OK bool `json:"ok"`
	}
	err := ai.CompleteJSON(ctx, c, "You are a connectivity check.", `Return {"ok": true}.`, &out)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": out.OK, "ms": time.Since(start).Milliseconds()})
}

func (s *Server) jobsProviderModels(w http.ResponseWriter, r *http.Request) {
	c, ok := s.providerClient(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	ids, err := c.Models(ctx)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	slices.Sort(ids)
	writeJSON(w, http.StatusOK, ids)
}

// ── accounts ─────────────────────────────────────────────────────────────────

func portalParam(w http.ResponseWriter, r *http.Request) (string, bool) {
	p := chi.URLParam(r, "portal")
	if _, ok := jobs.PortalByName(p); !ok {
		writeError(w, http.StatusNotFound, "Unknown portal.")
		return "", false
	}
	return p, true
}

func (s *Server) jobsAccounts(w http.ResponseWriter, r *http.Request) {
	list, err := s.jobs.Repo.Accounts(r.Context())
	if jobsFail(w, err, "accounts") {
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) putJobsAccount(w http.ResponseWriter, r *http.Request) {
	portal, ok := portalParam(w, r)
	if !ok {
		return
	}
	var in struct {
		Enabled bool `json:"enabled"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	if jobsFail(w, s.jobs.Repo.SetAccountEnabled(r.Context(), portal, in.Enabled), "account") {
		return
	}
	s.jobsAccounts(w, r)
}

// importJobsCookies accepts the JSON array a cookie-export extension
// produces (Cookie-Editor, EditThisCookie) and stores the portal's cookies.
func (s *Server) importJobsCookies(w http.ResponseWriter, r *http.Request) {
	portal, ok := portalParam(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "The cookie file is too large.")
		return
	}
	var wrapped struct {
		Cookies json.RawMessage `json:"cookies"`
	}
	if json.Unmarshal(raw, &wrapped) == nil && len(wrapped.Cookies) > 0 {
		raw = wrapped.Cookies
	}
	var cookies []jobs.Cookie
	if err := json.Unmarshal(raw, &cookies); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "Paste the cookies as the JSON array your cookie-export extension produces.")
		return
	}
	p, _ := jobs.PortalByName(portal)
	var keep []jobs.Cookie
	for _, c := range cookies {
		d := strings.TrimPrefix(c.Domain, ".")
		for _, suffix := range p.CookieDomains() {
			if d == suffix || strings.HasSuffix(d, "."+suffix) {
				keep = append(keep, c)
				break
			}
		}
	}
	if len(keep) == 0 {
		writeError(w, http.StatusUnprocessableEntity, "None of those cookies belong to "+jobs.PortalLabel[portal]+". Export them while on the portal's site.")
		return
	}
	if jobsFail(w, s.jobs.Repo.SaveCookies(r.Context(), portal, keep), "account") {
		return
	}
	_ = s.jobs.Repo.SetAccountStatus(r.Context(), portal, "connected", "Cookies imported; checking the session…")
	_, _ = s.jobs.Repo.Enqueue(r.Context(), "check", nil, portal, "manual")
	s.jobsAccounts(w, r)
}

func (s *Server) clearJobsCookies(w http.ResponseWriter, r *http.Request) {
	portal, ok := portalParam(w, r)
	if !ok {
		return
	}
	if jobsFail(w, s.jobs.Repo.ClearCookies(r.Context(), portal), "account") {
		return
	}
	s.jobsAccounts(w, r)
}

func (s *Server) jobsAccountTask(kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		portal, ok := portalParam(w, r)
		if !ok {
			return
		}
		if _, err := s.jobs.Repo.Enqueue(r.Context(), kind, nil, portal, "manual"); jobsFail(w, err, "task") {
			return
		}
		w.WriteHeader(http.StatusAccepted)
	}
}

// ── searches ─────────────────────────────────────────────────────────────────

func (s *Server) jobsSearches(w http.ResponseWriter, r *http.Request) {
	list, err := s.jobs.Repo.Searches(r.Context())
	if jobsFail(w, err, "searches") {
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func cleanList(in []string, allowed []string) []string {
	out := []string{}
	for _, v := range in {
		v = strings.TrimSpace(v)
		if v == "" || slices.Contains(out, v) {
			continue
		}
		if allowed != nil && !slices.Contains(allowed, v) {
			continue
		}
		out = append(out, v)
	}
	return out
}

func validateSearch(sr *jobs.Search) string {
	sr.Name = strings.TrimSpace(sr.Name)
	sr.Keywords = cleanList(sr.Keywords, nil)
	sr.ExcludeKeywords = cleanList(sr.ExcludeKeywords, nil)
	sr.ExcludeCompanies = cleanList(sr.ExcludeCompanies, nil)
	sr.Locations = cleanList(sr.Locations, nil)
	sr.Portals = cleanList(sr.Portals, jobs.Portals)
	sr.WorkModes = cleanList(sr.WorkModes, jobs.WorkModes)
	sr.JobTypes = cleanList(sr.JobTypes, jobs.JobTypes)
	sr.Seniority = cleanList(sr.Seniority, jobs.Seniority)
	sr.Schedule = strings.Join(strings.Fields(sr.Schedule), " ")
	switch {
	case sr.Name == "":
		return "Give the search a name."
	case len(sr.Keywords) == 0:
		return "Add at least one keyword, e.g. “Golang developer”."
	case len(sr.Portals) == 0:
		return "Choose at least one portal."
	case sr.MinScore < 0 || sr.MinScore > 100 || sr.AutoMinScore < 0 || sr.AutoMinScore > 100:
		return "Scores are between 0 and 100."
	case sr.AutoApply && sr.AutoMinScore < sr.MinScore:
		return "The auto-apply score must be at least the review-queue score."
	case sr.MaxResults < 1 || sr.MaxResults > 200:
		return "Results per run must be between 1 and 200."
	case sr.PostedWithinDays < 0 || sr.PostedWithinDays > 60:
		return "“Posted within” must be 0–60 days (0 means any time)."
	case sr.DailyCap < 0 || sr.DailyCap > 200:
		return "The daily cap must be between 0 and 200."
	}
	if _, err := jobs.NextRun(sr.Schedule, "UTC", time.Now()); err != nil {
		return err.Error()
	}
	return ""
}

func (s *Server) saveJobsSearch(update bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in jobs.Search
		if !readJSON(w, r, &in) {
			return
		}
		in.ID = 0
		if update {
			id, ok := idParam(w, r)
			if !ok {
				return
			}
			in.ID = id
		}
		if msg := validateSearch(&in); msg != "" {
			writeError(w, http.StatusUnprocessableEntity, msg)
			return
		}
		set, err := s.jobs.Repo.Settings(r.Context())
		if jobsFail(w, err, "search") {
			return
		}
		in.NextRunAt = nil
		if in.Enabled {
			if next, err := jobs.NextRun(in.Schedule, set.Timezone, time.Now()); err == nil {
				in.NextRunAt = &next
			}
		}
		out, err := s.jobs.Repo.SaveSearch(r.Context(), in)
		if jobsFail(w, err, "search") {
			return
		}
		status := http.StatusOK
		if !update {
			status = http.StatusCreated
		}
		writeJSON(w, status, out)
	}
}

func (s *Server) deleteJobsSearch(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	if jobsFail(w, s.jobs.Repo.DeleteSearch(r.Context(), id), "search") {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) runJobsSearch(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	n, err := s.jobs.RunSearch(r.Context(), id, "manual")
	if jobsFail(w, err, "search") {
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]int{"queued": n})
}

// ── listings & applications ──────────────────────────────────────────────────

func (s *Server) jobsListings(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := jobs.ListingFilter{Portal: q.Get("portal"), Query: q.Get("q")}
	if v := q.Get("status"); v != "" {
		f.Statuses = strings.Split(v, ",")
	}
	f.SearchID, _ = strconv.ParseInt(q.Get("search"), 10, 64)
	f.Limit, _ = strconv.Atoi(q.Get("limit"))
	f.Offset, _ = strconv.Atoi(q.Get("offset"))
	list, total, err := s.jobs.Repo.Listings(r.Context(), f)
	if jobsFail(w, err, "listings") {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": list, "total": total})
}

func (s *Server) jobsListing(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	l, err := s.jobs.Repo.Listing(r.Context(), id)
	if jobsFail(w, err, "job") {
		return
	}
	apps, err := s.jobs.Repo.ApplicationsFor(r.Context(), id)
	if jobsFail(w, err, "job") {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"listing": l, "applications": apps})
}

func (s *Server) reviewJobsListing(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	var in struct {
		Approve bool `json:"approve"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	l, err := s.jobs.Repo.Review(r.Context(), id, in.Approve)
	if errors.Is(err, jobs.ErrNotFound) || err == nil {
		if jobsFail(w, err, "job") {
			return
		}
		writeJSON(w, http.StatusOK, l)
		return
	}
	writeError(w, http.StatusConflict, err.Error())
}

func (s *Server) bulkReviewJobs(w http.ResponseWriter, r *http.Request) {
	var in struct {
		IDs     []int64 `json:"ids"`
		Approve bool    `json:"approve"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	done := 0
	for _, id := range in.IDs {
		if _, err := s.jobs.Repo.Review(r.Context(), id, in.Approve); err == nil {
			done++
		}
	}
	writeJSON(w, http.StatusOK, map[string]int{"updated": done})
}

func (s *Server) answerJobsListing(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	var in struct {
		Answers []jobs.Answer `json:"answers"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	requeued, err := s.jobs.AnswerQuestions(r.Context(), id, in.Answers)
	if jobsFail(w, err, "job") {
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"requeued": requeued})
}

// markJobsApplied records an application made by hand (e.g. on an external site).
func (s *Server) markJobsApplied(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	if jobsFail(w, s.jobs.Repo.SetListingStatus(r.Context(), id, jobs.StatusApplied, "Marked as applied by hand."), "job") {
		return
	}
	s.jobsListing(w, r)
}

func (s *Server) jobsApplications(w http.ResponseWriter, r *http.Request) {
	var statuses []string
	if v := r.URL.Query().Get("status"); v != "" {
		statuses = strings.Split(v, ",")
	}
	list, err := s.jobs.Repo.Applications(r.Context(), statuses, 150)
	if jobsFail(w, err, "applications") {
		return
	}
	writeJSON(w, http.StatusOK, list)
}

// ── answer bank ──────────────────────────────────────────────────────────────

func (s *Server) jobsAnswers(w http.ResponseWriter, r *http.Request) {
	list, err := s.jobs.Repo.Answers(r.Context())
	if jobsFail(w, err, "answers") {
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) saveJobsAnswer(update bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in jobs.SavedAnswer
		if !readJSON(w, r, &in) {
			return
		}
		in.ID = 0
		if update {
			id, ok := idParam(w, r)
			if !ok {
				return
			}
			in.ID = id
		}
		in.Question, in.Answer = strings.TrimSpace(in.Question), strings.TrimSpace(in.Answer)
		if in.Question == "" {
			writeError(w, http.StatusUnprocessableEntity, "Write the question as the form asks it.")
			return
		}
		// Anything saved from the dashboard is the user's own answer.
		in.Source, in.Approved = "manual", true
		out, err := s.jobs.Repo.SaveAnswer(r.Context(), in)
		if jobsFail(w, err, "answer") {
			return
		}
		writeJSON(w, http.StatusOK, out)
	}
}

func (s *Server) deleteJobsAnswer(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	if jobsFail(w, s.jobs.Repo.DeleteAnswer(r.Context(), id), "answer") {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── tasks & files ────────────────────────────────────────────────────────────

func (s *Server) jobsTasks(w http.ResponseWriter, r *http.Request) {
	list, err := s.jobs.Repo.Tasks(r.Context(), 100)
	if jobsFail(w, err, "activity") {
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) cancelJobsTask(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	if jobsFail(w, s.jobs.Repo.CancelTask(r.Context(), id), "task") {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// jobsFile streams a worker screenshot. Unlike /media these are private.
func (s *Server) jobsFile(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "*")
	if !jobs.FileKey(key) {
		http.NotFound(w, r)
		return
	}
	obj, err := s.store.Get(r.Context(), key)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "file unavailable", http.StatusBadGateway)
		return
	}
	defer obj.Close()
	w.Header().Set("Content-Type", obj.ContentType)
	w.Header().Set("Cache-Control", "private, max-age=86400")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	_, _ = io.Copy(w, obj)
}
