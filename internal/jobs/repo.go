package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/devileya/my-portfolio/internal/ai"
	"github.com/devileya/my-portfolio/internal/secrets"
)

var ErrNotFound = errors.New("not found")

type Repo struct {
	db  *pgxpool.Pool
	box *secrets.Box
}

func NewRepo(db *pgxpool.Pool, box *secrets.Box) *Repo { return &Repo{db: db, box: box} }

func (r *Repo) SecretsReady() bool { return r.box.Ready() }

func notFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

func nonNil(lists ...*[]string) {
	for _, l := range lists {
		if *l == nil {
			*l = []string{}
		}
	}
}

// ── settings ─────────────────────────────────────────────────────────────────

func (r *Repo) Settings(ctx context.Context) (Settings, error) {
	var s Settings
	err := r.db.QueryRow(ctx, `SELECT paused, dry_run, timezone, apply_start_hour, apply_end_hour,
		min_gap_minutes, max_gap_minutes, portal_caps, score_provider_id, apply_provider_id, applicant,
		cover_letter_style, notify_matches, notify_applied, notify_digest, digest_hour,
		telegram_token_sealed IS NOT NULL, telegram_token_hint, telegram_bot_name, telegram_chat_id,
		telegram_chat_name, telegram_poll,
		worker_name, worker_state, worker_seen_at, updated_at FROM job_settings WHERE id = 1`).Scan(
		&s.Paused, &s.DryRun, &s.Timezone, &s.ApplyStartHour, &s.ApplyEndHour,
		&s.MinGapMinutes, &s.MaxGapMinutes, &s.PortalCaps, &s.ScoreProviderID, &s.ApplyProviderID, &s.Applicant,
		&s.CoverLetterStyle, &s.NotifyMatches, &s.NotifyApplied, &s.NotifyDigest, &s.DigestHour,
		&s.Telegram.HasToken, &s.Telegram.TokenHint, &s.Telegram.BotName, &s.Telegram.ChatID,
		&s.Telegram.ChatName, &s.Telegram.Poll,
		&s.WorkerName, &s.WorkerState, &s.WorkerSeenAt, &s.UpdatedAt)
	if s.PortalCaps == nil {
		s.PortalCaps = map[string]int{}
	}
	if s.Applicant == nil {
		s.Applicant = map[string]string{}
	}
	return s, err
}

func (r *Repo) SaveSettings(ctx context.Context, s Settings) error {
	if s.PortalCaps == nil {
		s.PortalCaps = map[string]int{}
	}
	if s.Applicant == nil {
		s.Applicant = map[string]string{}
	}
	_, err := r.db.Exec(ctx, `UPDATE job_settings SET paused=$1, dry_run=$2, timezone=$3,
		apply_start_hour=$4, apply_end_hour=$5, min_gap_minutes=$6, max_gap_minutes=$7, portal_caps=$8,
		score_provider_id=$9, apply_provider_id=$10, applicant=$11, cover_letter_style=$12,
		notify_matches=$13, notify_applied=$14, notify_digest=$15, digest_hour=$16, updated_at=now()
		WHERE id = 1`,
		s.Paused, s.DryRun, s.Timezone, s.ApplyStartHour, s.ApplyEndHour, s.MinGapMinutes, s.MaxGapMinutes,
		s.PortalCaps, s.ScoreProviderID, s.ApplyProviderID, s.Applicant, s.CoverLetterStyle,
		s.NotifyMatches, s.NotifyApplied, s.NotifyDigest, s.DigestHour)
	return err
}

// TelegramToken returns the opened bot token and chat, or "" when unset.
func (r *Repo) TelegramToken(ctx context.Context) (string, int64, error) {
	var sealed []byte
	var chat int64
	if err := r.db.QueryRow(ctx, `SELECT telegram_token_sealed, telegram_chat_id FROM job_settings WHERE id = 1`).Scan(&sealed, &chat); err != nil {
		return "", 0, err
	}
	if sealed == nil {
		return "", chat, nil
	}
	token, err := r.box.Open(sealed)
	return string(token), chat, err
}

// SaveTelegramToken stores a verified token. A new bot starts without a chat.
func (r *Repo) SaveTelegramToken(ctx context.Context, token, botName string) error {
	sealed, err := r.box.Seal([]byte(token))
	if err != nil {
		return err
	}
	_, err = r.db.Exec(ctx, `UPDATE job_settings SET telegram_token_sealed=$1, telegram_token_hint=$2,
		telegram_bot_name=$3, telegram_chat_id=0, telegram_chat_name='', updated_at=now() WHERE id = 1`,
		sealed, secrets.Hint(token), botName)
	return err
}

func (r *Repo) SaveTelegramChat(ctx context.Context, chatID int64, chatName string, poll bool) error {
	_, err := r.db.Exec(ctx, `UPDATE job_settings SET telegram_chat_id=$1, telegram_chat_name=$2, telegram_poll=$3,
		updated_at=now() WHERE id = 1`, chatID, chatName, poll)
	return err
}

func (r *Repo) ClearTelegram(ctx context.Context) error {
	_, err := r.db.Exec(ctx, `UPDATE job_settings SET telegram_token_sealed=NULL, telegram_token_hint='',
		telegram_bot_name='', telegram_chat_id=0, telegram_chat_name='', updated_at=now() WHERE id = 1`)
	return err
}

func (r *Repo) SetPaused(ctx context.Context, paused bool) error {
	_, err := r.db.Exec(ctx, `UPDATE job_settings SET paused=$1, updated_at=now() WHERE id = 1`, paused)
	return err
}

func (r *Repo) Heartbeat(ctx context.Context, name, state string) error {
	_, err := r.db.Exec(ctx, `UPDATE job_settings SET worker_name=$1, worker_state=$2, worker_seen_at=now() WHERE id = 1`, name, state)
	return err
}

// ClaimDigest records that today's digest is going out; false if it already did.
func (r *Repo) ClaimDigest(ctx context.Context, day time.Time) (bool, error) {
	tag, err := r.db.Exec(ctx, `UPDATE job_settings SET last_digest_on=$1
		WHERE id = 1 AND (last_digest_on IS NULL OR last_digest_on < $1)`, day.Format("2006-01-02"))
	return tag.RowsAffected() == 1, err
}

// ── AI providers ─────────────────────────────────────────────────────────────

const providerCols = `id, label, kind, base_url, model, key_hint, key_sealed IS NOT NULL, created_at, updated_at`

func scanProvider(row pgx.Row) (Provider, error) {
	var p Provider
	err := row.Scan(&p.ID, &p.Label, &p.Kind, &p.BaseURL, &p.Model, &p.KeyHint, &p.HasKey, &p.CreatedAt, &p.UpdatedAt)
	return p, err
}

func (r *Repo) Providers(ctx context.Context) ([]Provider, error) {
	rows, err := r.db.Query(ctx, `SELECT `+providerCols+` FROM job_ai_providers ORDER BY id`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Provider, error) { return scanProvider(row) })
}

// SaveProvider creates or updates a provider. An empty APIKey on update keeps
// the stored key.
func (r *Repo) SaveProvider(ctx context.Context, p Provider) (Provider, error) {
	var sealed []byte
	hint := ""
	if key := strings.TrimSpace(p.APIKey); key != "" {
		var err error
		if sealed, err = r.box.Seal([]byte(key)); err != nil {
			return p, err
		}
		hint = secrets.Hint(key)
	}
	if p.ID == 0 {
		return scanProvider(r.db.QueryRow(ctx, `INSERT INTO job_ai_providers (label, kind, base_url, model, key_sealed, key_hint)
			VALUES ($1,$2,$3,$4,$5,$6) RETURNING `+providerCols, p.Label, p.Kind, p.BaseURL, p.Model, sealed, hint))
	}
	out, err := scanProvider(r.db.QueryRow(ctx, `UPDATE job_ai_providers SET label=$2, kind=$3, base_url=$4, model=$5,
		key_sealed = COALESCE($6, key_sealed), key_hint = CASE WHEN $6::bytea IS NULL THEN key_hint ELSE $7 END,
		updated_at=now() WHERE id=$1 RETURNING `+providerCols, p.ID, p.Label, p.Kind, p.BaseURL, p.Model, sealed, hint))
	return out, notFound(err)
}

func (r *Repo) DeleteProvider(ctx context.Context, id int64) error {
	tag, err := r.db.Exec(ctx, `DELETE FROM job_ai_providers WHERE id=$1`, id)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	_, _ = r.db.Exec(ctx, `UPDATE job_settings SET
		score_provider_id = NULLIF(score_provider_id, $1), apply_provider_id = NULLIF(apply_provider_id, $1)`, id)
	return err
}

// AIProvider returns the provider with its key opened, ready for ai.New.
func (r *Repo) AIProvider(ctx context.Context, id int64) (ai.Provider, error) {
	var p ai.Provider
	var sealed []byte
	err := r.db.QueryRow(ctx, `SELECT kind, base_url, model, key_sealed FROM job_ai_providers WHERE id=$1`, id).
		Scan(&p.Kind, &p.BaseURL, &p.Model, &sealed)
	if err != nil {
		return p, notFound(err)
	}
	if sealed == nil {
		return p, errors.New("this AI provider has no API key")
	}
	key, err := r.box.Open(sealed)
	if err != nil {
		return p, err
	}
	p.APIKey = string(key)
	return p, nil
}

// ── accounts ─────────────────────────────────────────────────────────────────

func (r *Repo) Accounts(ctx context.Context) ([]Account, error) {
	rows, err := r.db.Query(ctx, `SELECT portal, enabled, status, status_note, cookies_count, checked_at, updated_at
		FROM job_accounts ORDER BY array_position(ARRAY['linkedin','indeed','glints','kalibrr'], portal)`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Account, error) {
		var a Account
		err := row.Scan(&a.Portal, &a.Enabled, &a.Status, &a.StatusNote, &a.CookiesCount, &a.CheckedAt, &a.UpdatedAt)
		a.Label = PortalLabel[a.Portal]
		return a, err
	})
}

func (r *Repo) Account(ctx context.Context, portal string) (Account, error) {
	all, err := r.Accounts(ctx)
	if err != nil {
		return Account{}, err
	}
	for _, a := range all {
		if a.Portal == portal {
			return a, nil
		}
	}
	return Account{}, ErrNotFound
}

func (r *Repo) SetAccountEnabled(ctx context.Context, portal string, enabled bool) error {
	tag, err := r.db.Exec(ctx, `UPDATE job_accounts SET enabled=$2, updated_at=now() WHERE portal=$1`, portal, enabled)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

func (r *Repo) SetAccountStatus(ctx context.Context, portal, status, note string) error {
	_, err := r.db.Exec(ctx, `UPDATE job_accounts SET status=$2, status_note=$3, checked_at=now(), updated_at=now()
		WHERE portal=$1`, portal, status, note)
	return err
}

func (r *Repo) SaveCookies(ctx context.Context, portal string, cookies []Cookie) error {
	data, err := json.Marshal(cookies)
	if err != nil {
		return err
	}
	sealed, err := r.box.Seal(data)
	if err != nil {
		return err
	}
	_, err = r.db.Exec(ctx, `UPDATE job_accounts SET cookies_sealed=$2, cookies_count=$3, updated_at=now() WHERE portal=$1`,
		portal, sealed, len(cookies))
	return err
}

func (r *Repo) ClearCookies(ctx context.Context, portal string) error {
	_, err := r.db.Exec(ctx, `UPDATE job_accounts SET cookies_sealed=NULL, cookies_count=0, status='disconnected',
		status_note='', updated_at=now() WHERE portal=$1`, portal)
	return err
}

func (r *Repo) Cookies(ctx context.Context, portal string) ([]Cookie, error) {
	var sealed []byte
	if err := r.db.QueryRow(ctx, `SELECT cookies_sealed FROM job_accounts WHERE portal=$1`, portal).Scan(&sealed); err != nil {
		return nil, notFound(err)
	}
	if sealed == nil {
		return nil, nil
	}
	data, err := r.box.Open(sealed)
	if err != nil {
		return nil, err
	}
	var out []Cookie
	return out, json.Unmarshal(data, &out)
}

// ── searches ─────────────────────────────────────────────────────────────────

const searchCols = `id, name, enabled, portals, keywords, exclude_keywords, exclude_companies, locations,
	work_modes, job_types, seniority, min_salary, posted_within_days, easy_apply_only, schedule, max_results,
	min_score, auto_apply, auto_min_score, daily_cap, notes, last_run_at, next_run_at, created_at, updated_at`

func scanSearch(row pgx.Row) (Search, error) {
	var s Search
	err := row.Scan(&s.ID, &s.Name, &s.Enabled, &s.Portals, &s.Keywords, &s.ExcludeKeywords, &s.ExcludeCompanies,
		&s.Locations, &s.WorkModes, &s.JobTypes, &s.Seniority, &s.MinSalary, &s.PostedWithinDays, &s.EasyApplyOnly,
		&s.Schedule, &s.MaxResults, &s.MinScore, &s.AutoApply, &s.AutoMinScore, &s.DailyCap, &s.Notes,
		&s.LastRunAt, &s.NextRunAt, &s.CreatedAt, &s.UpdatedAt)
	nonNil(&s.Portals, &s.Keywords, &s.ExcludeKeywords, &s.ExcludeCompanies, &s.Locations, &s.WorkModes, &s.JobTypes, &s.Seniority)
	return s, err
}

func (r *Repo) Searches(ctx context.Context) ([]Search, error) {
	rows, err := r.db.Query(ctx, `SELECT `+searchCols+` FROM job_searches ORDER BY id`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Search, error) { return scanSearch(row) })
}

func (r *Repo) Search(ctx context.Context, id int64) (Search, error) {
	s, err := scanSearch(r.db.QueryRow(ctx, `SELECT `+searchCols+` FROM job_searches WHERE id=$1`, id))
	return s, notFound(err)
}

func (r *Repo) SaveSearch(ctx context.Context, s Search) (Search, error) {
	nonNil(&s.Portals, &s.Keywords, &s.ExcludeKeywords, &s.ExcludeCompanies, &s.Locations, &s.WorkModes, &s.JobTypes, &s.Seniority)
	args := []any{s.Name, s.Enabled, s.Portals, s.Keywords, s.ExcludeKeywords, s.ExcludeCompanies, s.Locations,
		s.WorkModes, s.JobTypes, s.Seniority, s.MinSalary, s.PostedWithinDays, s.EasyApplyOnly, s.Schedule,
		s.MaxResults, s.MinScore, s.AutoApply, s.AutoMinScore, s.DailyCap, s.Notes, s.NextRunAt}
	if s.ID == 0 {
		return scanSearch(r.db.QueryRow(ctx, `INSERT INTO job_searches (name, enabled, portals, keywords,
			exclude_keywords, exclude_companies, locations, work_modes, job_types, seniority, min_salary,
			posted_within_days, easy_apply_only, schedule, max_results, min_score, auto_apply, auto_min_score,
			daily_cap, notes, next_run_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21)
			RETURNING `+searchCols, args...))
	}
	out, err := scanSearch(r.db.QueryRow(ctx, `UPDATE job_searches SET name=$1, enabled=$2, portals=$3, keywords=$4,
		exclude_keywords=$5, exclude_companies=$6, locations=$7, work_modes=$8, job_types=$9, seniority=$10,
		min_salary=$11, posted_within_days=$12, easy_apply_only=$13, schedule=$14, max_results=$15, min_score=$16,
		auto_apply=$17, auto_min_score=$18, daily_cap=$19, notes=$20, next_run_at=$21, updated_at=now()
		WHERE id=$22 RETURNING `+searchCols, append(args, s.ID)...))
	return out, notFound(err)
}

func (r *Repo) DeleteSearch(ctx context.Context, id int64) error {
	tag, err := r.db.Exec(ctx, `DELETE FROM job_searches WHERE id=$1`, id)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

func (r *Repo) SetNextRun(ctx context.Context, id int64, next *time.Time, ran bool) error {
	_, err := r.db.Exec(ctx, `UPDATE job_searches SET next_run_at=$2,
		last_run_at = CASE WHEN $3 THEN now() ELSE last_run_at END WHERE id=$1`, id, next, ran)
	return err
}

// ── listings ─────────────────────────────────────────────────────────────────

const listingCols = `l.id, l.portal, l.external_id, l.search_id, COALESCE(s.name, ''), l.url, l.title, l.company,
	l.location, l.work_mode, l.job_type, l.salary, l.salary_min, l.salary_max, l.salary_currency, l.salary_period, l.description, l.easy_apply, l.posted_at, l.score, l.score_reason,
	l.score_details, l.status, l.status_note, l.attempts, l.approved_at, l.applied_at, l.found_at, l.updated_at`

const listingFrom = ` FROM job_listings l LEFT JOIN job_searches s ON s.id = l.search_id`

func scanListing(row pgx.Row) (Listing, error) {
	var l Listing
	err := row.Scan(&l.ID, &l.Portal, &l.ExternalID, &l.SearchID, &l.SearchName, &l.URL, &l.Title, &l.Company,
		&l.Location, &l.WorkMode, &l.JobType, &l.Salary, &l.SalaryMin, &l.SalaryMax, &l.SalaryCurrency, &l.SalaryPeriod, &l.Description, &l.EasyApply, &l.PostedAt, &l.Score,
		&l.ScoreReason, &l.ScoreDetails, &l.Status, &l.StatusNote, &l.Attempts, &l.ApprovedAt, &l.AppliedAt,
		&l.FoundAt, &l.UpdatedAt)
	return l, err
}

// InsertListing stores a listing seen for the first time. It returns false
// (and leaves the row alone) when the portal's job was already known.
func (r *Repo) InsertListing(ctx context.Context, l *Listing) (bool, error) {
	l.fillSalary()
	err := r.db.QueryRow(ctx, `INSERT INTO job_listings (portal, external_id, search_id, url, title, company, location,
		work_mode, job_type, salary, salary_min, salary_max, salary_currency, salary_period,
		description, easy_apply, posted_at, fingerprint)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)
		ON CONFLICT (portal, external_id) DO NOTHING RETURNING id, found_at`,
		l.Portal, l.ExternalID, l.SearchID, l.URL, l.Title, l.Company, l.Location, l.WorkMode, l.JobType, l.Salary,
		l.SalaryMin, l.SalaryMax, l.SalaryCurrency, l.SalaryPeriod,
		l.Description, l.EasyApply, l.PostedAt, Fingerprint(l.Company, l.Title)).Scan(&l.ID, &l.FoundAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	l.Status = StatusNew
	return err == nil, err
}

func (r *Repo) UpdateListingDetails(ctx context.Context, l Listing) error {
	l.fillSalary()
	_, err := r.db.Exec(ctx, `UPDATE job_listings SET description=$2, easy_apply=$3, work_mode=$4, job_type=$5,
		salary=$6, location=$7, salary_min=$8, salary_max=$9, salary_currency=$10, salary_period=$11,
		updated_at=now() WHERE id=$1`,
		l.ID, l.Description, l.EasyApply, l.WorkMode, l.JobType, l.Salary, l.Location,
		l.SalaryMin, l.SalaryMax, l.SalaryCurrency, l.SalaryPeriod)
	return err
}

func (r *Repo) SetScore(ctx context.Context, id int64, score int, reason string, d ScoreDetails, status string) error {
	_, err := r.db.Exec(ctx, `UPDATE job_listings SET score=$2, score_reason=$3, score_details=$4, status=$5,
		approved_at = CASE WHEN $5 = 'approved' THEN now() ELSE approved_at END, updated_at=now() WHERE id=$1`,
		id, score, reason, d, status)
	return err
}

func (r *Repo) SetListingStatus(ctx context.Context, id int64, status, note string) error {
	tag, err := r.db.Exec(ctx, `UPDATE job_listings SET status=$2, status_note=$3,
		approved_at = CASE WHEN $2 = 'approved' THEN now() ELSE approved_at END,
		applied_at = CASE WHEN $2 = 'applied' THEN now() ELSE applied_at END,
		updated_at=now() WHERE id=$1`, id, status, note)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// Review moves a queued (or stuck) listing to approved or skipped. Listings
// already applied to can't be reviewed again.
func (r *Repo) Review(ctx context.Context, id int64, approve bool) (Listing, error) {
	status := StatusSkipped
	if approve {
		status = StatusApproved
	}
	tag, err := r.db.Exec(ctx, `UPDATE job_listings SET status=$2, status_note='',
		approved_at = CASE WHEN $2 = 'approved' THEN now() ELSE approved_at END, updated_at=now()
		WHERE id=$1 AND status NOT IN ('applied', 'applying')`, id, status)
	if err != nil {
		return Listing{}, err
	}
	l, err := r.Listing(ctx, id)
	if err == nil && tag.RowsAffected() == 0 {
		return l, fmt.Errorf("this job is already %s", strings.ReplaceAll(l.Status, "_", " "))
	}
	return l, err
}

type ListingFilter struct {
	Statuses []string
	Portal   string
	SearchID int64
	Query    string
	Limit    int
	Offset   int
}

func (r *Repo) Listings(ctx context.Context, f ListingFilter) ([]Listing, int, error) {
	where := []string{"TRUE"}
	var args []any
	add := func(cond string, v any) {
		args = append(args, v)
		where = append(where, fmt.Sprintf(cond, len(args)))
	}
	if len(f.Statuses) > 0 {
		add("l.status = ANY($%d)", f.Statuses)
	}
	if f.Portal != "" {
		add("l.portal = $%d", f.Portal)
	}
	if f.SearchID != 0 {
		add("l.search_id = $%d", f.SearchID)
	}
	if q := strings.TrimSpace(f.Query); q != "" {
		add("(l.title ILIKE $%[1]d OR l.company ILIKE $%[1]d OR l.location ILIKE $%[1]d)", "%"+q+"%")
	}
	cond := strings.Join(where, " AND ")
	var total int
	if err := r.db.QueryRow(ctx, `SELECT count(*)`+listingFrom+` WHERE `+cond, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	if f.Limit <= 0 || f.Limit > 200 {
		f.Limit = 50
	}
	args = append(args, f.Limit, f.Offset)
	rows, err := r.db.Query(ctx, `SELECT `+listingCols+listingFrom+` WHERE `+cond+
		fmt.Sprintf(` ORDER BY l.score DESC NULLS LAST, l.found_at DESC LIMIT $%d OFFSET $%d`, len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Listing, error) {
		l, err := scanListing(row)
		l.Description = "" // lists stay small; the detail view loads it
		return l, err
	})
	return list, total, err
}

func (r *Repo) Listing(ctx context.Context, id int64) (Listing, error) {
	l, err := scanListing(r.db.QueryRow(ctx, `SELECT `+listingCols+listingFrom+` WHERE l.id=$1`, id))
	return l, notFound(err)
}

// DuplicateOf reports whether the same company+title was already applied to,
// approved or queued from any portal in the last 60 days.
func (r *Repo) DuplicateOf(ctx context.Context, id int64, fingerprint string) (bool, error) {
	if fingerprint == "|" || fingerprint == "" {
		return false, nil
	}
	var dup bool
	err := r.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM job_listings WHERE fingerprint=$1 AND id<>$2
		AND status IN ('queued','approved','applying','applied','needs_input','dry_run')
		AND found_at > now() - interval '60 days')`, fingerprint, id).Scan(&dup)
	return dup, err
}

// RecentlyAppliedCompany reports an application to the company in the last 30 days.
func (r *Repo) RecentlyAppliedCompany(ctx context.Context, company string) (bool, error) {
	if strings.TrimSpace(company) == "" {
		return false, nil
	}
	var yes bool
	err := r.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM job_listings WHERE lower(company)=lower($1)
		AND status='applied' AND applied_at > now() - interval '30 days')`, company).Scan(&yes)
	return yes, err
}

// ApprovedCandidates are approved listings in apply order: best score first.
func (r *Repo) ApprovedCandidates(ctx context.Context) ([]Listing, error) {
	rows, err := r.db.Query(ctx, `SELECT `+listingCols+listingFrom+` WHERE l.status='approved'
		ORDER BY l.score DESC NULLS LAST, l.approved_at LIMIT 100`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Listing, error) { return scanListing(row) })
}

// ClaimListing moves an approved listing to applying; false if someone else did.
func (r *Repo) ClaimListing(ctx context.Context, id int64) (bool, error) {
	tag, err := r.db.Exec(ctx, `UPDATE job_listings SET status='applying', attempts=attempts+1, updated_at=now()
		WHERE id=$1 AND status='approved'`, id)
	return tag.RowsAffected() == 1, err
}

func (r *Repo) AppliedSince(ctx context.Context, portal string, since time.Time) (int, error) {
	var n int
	err := r.db.QueryRow(ctx, `SELECT count(*) FROM job_applications a JOIN job_listings l ON l.id=a.listing_id
		WHERE ($1 = '' OR l.portal=$1) AND a.status IN ('submitted','running') AND a.started_at >= $2`, portal, since).Scan(&n)
	return n, err
}

func (r *Repo) AppliedForSearchSince(ctx context.Context, searchID int64, since time.Time) (int, error) {
	var n int
	err := r.db.QueryRow(ctx, `SELECT count(*) FROM job_applications a JOIN job_listings l ON l.id=a.listing_id
		WHERE l.search_id=$1 AND a.status IN ('submitted','running') AND a.started_at >= $2`, searchID, since).Scan(&n)
	return n, err
}

// LastAttemptAt is when the worker last started an application on portal.
func (r *Repo) LastAttemptAt(ctx context.Context, portal string) (*time.Time, error) {
	var t *time.Time
	err := r.db.QueryRow(ctx, `SELECT max(a.started_at) FROM job_applications a JOIN job_listings l ON l.id=a.listing_id
		WHERE l.portal=$1`, portal).Scan(&t)
	return t, err
}

// ── applications ─────────────────────────────────────────────────────────────

const appCols = `id, listing_id, status, answers, pending_questions, cover_letter, screenshot_key, error, steps,
	started_at, finished_at`

func scanApp(row pgx.Row) (Application, error) {
	var a Application
	err := row.Scan(&a.ID, &a.ListingID, &a.Status, &a.Answers, &a.PendingQuestions, &a.CoverLetter,
		&a.ScreenshotKey, &a.Error, &a.Steps, &a.StartedAt, &a.FinishedAt)
	if a.Answers == nil {
		a.Answers = []Answer{}
	}
	if a.PendingQuestions == nil {
		a.PendingQuestions = []Question{}
	}
	if a.ScreenshotKey != "" {
		a.ScreenshotURL = "/api/admin/jobs/files/" + a.ScreenshotKey
	}
	return a, err
}

func (r *Repo) StartApplication(ctx context.Context, listingID int64) (int64, error) {
	var id int64
	err := r.db.QueryRow(ctx, `INSERT INTO job_applications (listing_id) VALUES ($1) RETURNING id`, listingID).Scan(&id)
	return id, err
}

func (r *Repo) FinishApplication(ctx context.Context, a Application) error {
	if a.Answers == nil {
		a.Answers = []Answer{}
	}
	if a.PendingQuestions == nil {
		a.PendingQuestions = []Question{}
	}
	_, err := r.db.Exec(ctx, `UPDATE job_applications SET status=$2, answers=$3, pending_questions=$4,
		cover_letter=$5, screenshot_key=$6, error=$7, steps=$8, finished_at=now() WHERE id=$1`,
		a.ID, a.Status, a.Answers, a.PendingQuestions, a.CoverLetter, a.ScreenshotKey, a.Error, a.Steps)
	return err
}

func (r *Repo) Applications(ctx context.Context, statuses []string, limit int) ([]Application, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	rows, err := r.db.Query(ctx, `SELECT `+appCols+` FROM job_applications
		WHERE COALESCE(cardinality($1::text[]), 0) = 0 OR status = ANY($1) ORDER BY started_at DESC LIMIT $2`, statuses, limit)
	if err != nil {
		return nil, err
	}
	apps, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Application, error) { return scanApp(row) })
	if err != nil {
		return nil, err
	}
	return apps, r.attachListings(ctx, apps)
}

func (r *Repo) ApplicationsFor(ctx context.Context, listingID int64) ([]Application, error) {
	rows, err := r.db.Query(ctx, `SELECT `+appCols+` FROM job_applications WHERE listing_id=$1 ORDER BY started_at DESC`, listingID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Application, error) { return scanApp(row) })
}

func (r *Repo) LatestApplication(ctx context.Context, listingID int64) (Application, error) {
	a, err := scanApp(r.db.QueryRow(ctx, `SELECT `+appCols+` FROM job_applications WHERE listing_id=$1
		ORDER BY started_at DESC LIMIT 1`, listingID))
	return a, notFound(err)
}

func (r *Repo) attachListings(ctx context.Context, apps []Application) error {
	ids := make([]int64, 0, len(apps))
	for _, a := range apps {
		ids = append(ids, a.ListingID)
	}
	rows, err := r.db.Query(ctx, `SELECT `+listingCols+listingFrom+` WHERE l.id = ANY($1)`, ids)
	if err != nil {
		return err
	}
	list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Listing, error) {
		l, err := scanListing(row)
		l.Description = ""
		return l, err
	})
	if err != nil {
		return err
	}
	byID := map[int64]*Listing{}
	for i := range list {
		byID[list[i].ID] = &list[i]
	}
	for i := range apps {
		apps[i].Listing = byID[apps[i].ListingID]
	}
	return nil
}

// ── answer bank ──────────────────────────────────────────────────────────────

const answerCols = `id, key, question, answer, source, approved, used_count, updated_at`

func scanAnswer(row pgx.Row) (SavedAnswer, error) {
	var a SavedAnswer
	err := row.Scan(&a.ID, &a.Key, &a.Question, &a.Answer, &a.Source, &a.Approved, &a.UsedCount, &a.UpdatedAt)
	return a, err
}

func (r *Repo) Answers(ctx context.Context) ([]SavedAnswer, error) {
	rows, err := r.db.Query(ctx, `SELECT `+answerCols+` FROM job_answers ORDER BY approved, updated_at DESC`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (SavedAnswer, error) { return scanAnswer(row) })
}

// SaveAnswer upserts by normalized question. Answers saved by hand are
// approved; an AI suggestion never overwrites an approved answer.
func (r *Repo) SaveAnswer(ctx context.Context, a SavedAnswer) (SavedAnswer, error) {
	a.Key = QuestionKey(a.Question)
	if a.Source == "" {
		a.Source = "manual"
	}
	if a.ID != 0 {
		out, err := scanAnswer(r.db.QueryRow(ctx, `UPDATE job_answers SET key=$2, question=$3, answer=$4, source=$5,
			approved=$6, updated_at=now() WHERE id=$1 RETURNING `+answerCols, a.ID, a.Key, a.Question, a.Answer, a.Source, a.Approved))
		return out, notFound(err)
	}
	return scanAnswer(r.db.QueryRow(ctx, `INSERT INTO job_answers (key, question, answer, source, approved)
		VALUES ($1,$2,$3,$4,$5)
		ON CONFLICT (key) DO UPDATE SET question=EXCLUDED.question, answer=EXCLUDED.answer, source=EXCLUDED.source,
			approved=EXCLUDED.approved, updated_at=now()
		WHERE NOT (job_answers.approved AND EXCLUDED.source = 'ai')
		RETURNING `+answerCols, a.Key, a.Question, a.Answer, a.Source, a.Approved))
}

func (r *Repo) DeleteAnswer(ctx context.Context, id int64) error {
	tag, err := r.db.Exec(ctx, `DELETE FROM job_answers WHERE id=$1`, id)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// AnswerBank is every approved answer by key.
func (r *Repo) AnswerBank(ctx context.Context) (map[string]SavedAnswer, error) {
	rows, err := r.db.Query(ctx, `SELECT `+answerCols+` FROM job_answers WHERE approved`)
	if err != nil {
		return nil, err
	}
	list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (SavedAnswer, error) { return scanAnswer(row) })
	out := make(map[string]SavedAnswer, len(list))
	for _, a := range list {
		out[a.Key] = a
	}
	return out, err
}

func (r *Repo) MarkAnswersUsed(ctx context.Context, keys []string) {
	if len(keys) > 0 {
		_, _ = r.db.Exec(ctx, `UPDATE job_answers SET used_count=used_count+1 WHERE key = ANY($1)`, keys)
	}
}

// ── tasks ────────────────────────────────────────────────────────────────────

const taskCols = `t.id, t.kind, t.search_id, COALESCE(s.name, ''), t.portal, t.status, t.trigger, t.run_after,
	t.locked_by, t.started_at, t.finished_at, t.result, t.log, t.error, t.created_at`

func scanTask(row pgx.Row) (Task, error) {
	var t Task
	err := row.Scan(&t.ID, &t.Kind, &t.SearchID, &t.SearchName, &t.Portal, &t.Status, &t.Trigger, &t.RunAfter,
		&t.LockedBy, &t.StartedAt, &t.FinishedAt, &t.Result, &t.Log, &t.Error, &t.CreatedAt)
	return t, err
}

// Enqueue adds a task unless an identical one is already waiting.
func (r *Repo) Enqueue(ctx context.Context, kind string, searchID *int64, portal, trigger string) (bool, error) {
	tag, err := r.db.Exec(ctx, `INSERT INTO job_tasks (kind, search_id, portal, trigger)
		SELECT $1, $2, $3, $4 WHERE NOT EXISTS (SELECT 1 FROM job_tasks WHERE kind=$1
			AND search_id IS NOT DISTINCT FROM $2 AND portal=$3 AND status IN ('pending','running'))`,
		kind, searchID, portal, trigger)
	return tag.RowsAffected() == 1, err
}

// ClaimTask takes the next due task: logins first (someone is waiting at the
// browser window), then session checks, then searches.
func (r *Repo) ClaimTask(ctx context.Context, worker string) (Task, error) {
	var id int64
	err := r.db.QueryRow(ctx, `UPDATE job_tasks SET status='running', locked_by=$1, started_at=now()
		WHERE id = (SELECT id FROM job_tasks WHERE status='pending' AND run_after <= now()
			ORDER BY CASE kind WHEN 'login' THEN 0 WHEN 'check' THEN 1 ELSE 2 END, run_after
			LIMIT 1 FOR UPDATE SKIP LOCKED)
		RETURNING id`, worker).Scan(&id)
	if err != nil {
		return Task{}, notFound(err)
	}
	t, err := scanTask(r.db.QueryRow(ctx, `SELECT `+taskCols+` FROM job_tasks t LEFT JOIN job_searches s ON s.id=t.search_id
		WHERE t.id=$1`, id))
	return t, notFound(err)
}

func (r *Repo) FinishTask(ctx context.Context, id int64, status string, result any, log, errText string) error {
	if result == nil {
		result = map[string]any{}
	}
	_, err := r.db.Exec(ctx, `UPDATE job_tasks SET status=$2, result=$3, log=$4, error=$5, finished_at=now() WHERE id=$1`,
		id, status, result, log, errText)
	return err
}

func (r *Repo) CancelTask(ctx context.Context, id int64) error {
	tag, err := r.db.Exec(ctx, `UPDATE job_tasks SET status='cancelled', finished_at=now() WHERE id=$1 AND status='pending'`, id)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

func (r *Repo) Tasks(ctx context.Context, limit int) ([]Task, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := r.db.Query(ctx, `SELECT `+taskCols+` FROM job_tasks t LEFT JOIN job_searches s ON s.id=t.search_id
		ORDER BY t.created_at DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Task, error) { return scanTask(row) })
}

// RecoverStale releases work a crashed worker left behind.
func (r *Repo) RecoverStale(ctx context.Context) error {
	if _, err := r.db.Exec(ctx, `UPDATE job_tasks SET status='failed', error='The worker stopped before finishing.',
		finished_at=now() WHERE status='running' AND started_at < now() - interval '45 minutes'`); err != nil {
		return err
	}
	if _, err := r.db.Exec(ctx, `UPDATE job_applications SET status='failed', error='The worker stopped before finishing.',
		finished_at=now() WHERE status='running' AND started_at < now() - interval '30 minutes'`); err != nil {
		return err
	}
	_, err := r.db.Exec(ctx, `UPDATE job_listings SET status='approved', status_note='Retrying after an interrupted attempt.'
		WHERE status='applying' AND updated_at < now() - interval '30 minutes' AND attempts < 3`)
	return err
}

// ── telegram refs ────────────────────────────────────────────────────────────

type TelegramRef struct {
	MessageID   int64
	Kind        string
	ListingID   *int64
	QuestionKey string
	Question    string
}

func (r *Repo) SaveTelegramRef(ctx context.Context, ref TelegramRef) error {
	_, err := r.db.Exec(ctx, `INSERT INTO job_telegram_refs (message_id, kind, listing_id, question_key, question)
		VALUES ($1,$2,$3,$4,$5) ON CONFLICT (message_id) DO NOTHING`,
		ref.MessageID, ref.Kind, ref.ListingID, ref.QuestionKey, ref.Question)
	return err
}

func (r *Repo) TelegramRef(ctx context.Context, messageID int64) (TelegramRef, error) {
	var ref TelegramRef
	err := r.db.QueryRow(ctx, `SELECT message_id, kind, listing_id, question_key, question FROM job_telegram_refs
		WHERE message_id=$1`, messageID).Scan(&ref.MessageID, &ref.Kind, &ref.ListingID, &ref.QuestionKey, &ref.Question)
	return ref, notFound(err)
}

// ── overview ─────────────────────────────────────────────────────────────────

func (r *Repo) Overview(ctx context.Context) (Overview, error) {
	var o Overview
	var err error
	if o.Settings, err = r.Settings(ctx); err != nil {
		return o, err
	}
	if o.Accounts, err = r.Accounts(ctx); err != nil {
		return o, err
	}
	loc := o.Settings.Location()
	now := time.Now().In(loc)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)

	o.ByStatus = map[string]int{}
	rows, err := r.db.Query(ctx, `SELECT status, count(*) FROM job_listings GROUP BY status`)
	if err != nil {
		return o, err
	}
	for rows.Next() {
		var s string
		var n int
		if err := rows.Scan(&s, &n); err != nil {
			return o, err
		}
		o.ByStatus[s] = n
	}
	rows.Close()

	o.AppliedToday = map[string]int{}
	for _, p := range Portals {
		if o.AppliedToday[p], err = r.AppliedSince(ctx, p, today); err != nil {
			return o, err
		}
	}
	if o.AppliedWeek, err = r.AppliedSince(ctx, "", today.AddDate(0, 0, -6)); err != nil {
		return o, err
	}
	if err := r.db.QueryRow(ctx, `SELECT count(*), count(*) FILTER (WHERE enabled) FROM job_searches`).Scan(&o.Searches, &o.ActiveSearch); err != nil {
		return o, err
	}
	if err := r.db.QueryRow(ctx, `SELECT count(*) FROM job_tasks WHERE status IN ('pending','running')`).Scan(&o.PendingTasks); err != nil {
		return o, err
	}
	if err := r.db.QueryRow(ctx, `SELECT count(*) FROM job_ai_providers`).Scan(&o.Providers); err != nil {
		return o, err
	}
	if o.RecentTasks, err = r.Tasks(ctx, 8); err != nil {
		return o, err
	}

	o.TopQueued, _, err = r.Listings(ctx, ListingFilter{Statuses: []string{StatusQueued}, Limit: 5})
	return o, err
}

// FileKey reports whether key is a job-hunter object an admin may download.
func FileKey(key string) bool {
	return strings.HasPrefix(key, "jobs/") && !strings.Contains(key, "..")
}
