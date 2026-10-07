// Package content is the portfolio's data: profile, projects, timeline,
// skills, visitor messages and counters, all in Postgres.
package content

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/devileya/my-portfolio/internal/storage"
)

var ErrNotFound = errors.New("not found")

type Fact struct {
	Label string `json:"label"`
	Value string `json:"value"`
	URL   string `json:"url,omitempty"`
}

type Social struct {
	Label string `json:"label"`
	URL   string `json:"url"`
}

type CV struct {
	Filename  string     `json:"filename"`
	Size      int64      `json:"size"`
	SHA256    string     `json:"sha256"`
	UpdatedAt *time.Time `json:"updatedAt"`
	URL       string     `json:"url"`
}

type Profile struct {
	Name            string    `json:"name"`
	Role            string    `json:"role"`
	Headlines       []string  `json:"headlines"`
	Prompt          string    `json:"prompt"`
	Summary         string    `json:"summary"`
	Bio             string    `json:"bio"`
	Location        string    `json:"location"`
	Email           string    `json:"email"`
	YearsExperience int       `json:"yearsExperience"`
	Available       bool      `json:"available"`
	FocusAreas      []string  `json:"focusAreas"`
	Facts           []Fact    `json:"facts"`
	Socials         []Social  `json:"socials"`
	AvatarKey       string    `json:"avatarKey"`
	AvatarURL       string    `json:"avatarUrl"`
	CV              *CV       `json:"cv"`
	UpdatedAt       time.Time `json:"updatedAt"`

	cvKey string
}

type Project struct {
	ID          int64    `json:"id"`
	Title       string   `json:"title"`
	Category    string   `json:"category"`
	Platforms   string   `json:"platforms"`
	Summary     string   `json:"summary"`
	Description string   `json:"description"`
	Tech        []string `json:"tech"`
	ImageKey    string   `json:"imageKey"`
	ImageURL    string   `json:"imageUrl"`
	LiveURL     string   `json:"liveUrl"`
	AltURL      string   `json:"altUrl"`
	RepoURL     string   `json:"repoUrl"`
	Featured    bool     `json:"featured"`
	Published   bool     `json:"published"`
	SortOrder   int      `json:"sortOrder"`
}

type Experience struct {
	ID           int64    `json:"id"`
	Kind         string   `json:"kind"`
	Title        string   `json:"title"`
	Organization string   `json:"organization"`
	PeriodStart  string   `json:"periodStart"`
	PeriodEnd    string   `json:"periodEnd"`
	Summary      string   `json:"summary"`
	Highlights   []string `json:"highlights"`
	Tech         []string `json:"tech"`
	Published    bool     `json:"published"`
	SortOrder    int      `json:"sortOrder"`
}

type Skill struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Category  string `json:"category"`
	Level     string `json:"level"`
	SortOrder int    `json:"sortOrder"`
}

type Message struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	Body      string    `json:"body"`
	Read      bool      `json:"read"`
	CreatedAt time.Time `json:"createdAt"`
}

// Site is everything the public page renders, in one response.
type Site struct {
	Profile     Profile      `json:"profile"`
	Projects    []Project    `json:"projects"`
	Experiences []Experience `json:"experiences"`
	Skills      []Skill      `json:"skills"`
}

type Stats struct {
	CVDownloads    int64 `json:"cvDownloads"`
	SiteViews      int64 `json:"siteViews"`
	Projects       int64 `json:"projects"`
	UnreadMessages int64 `json:"unreadMessages"`
	Messages       int64 `json:"messages"`
}

func (p *Project) SetID(id int64)    { p.ID = id }
func (e *Experience) SetID(id int64) { e.ID = id }
func (s *Skill) SetID(id int64)      { s.ID = id }

type Repo struct{ db *pgxpool.Pool }

func NewRepo(db *pgxpool.Pool) *Repo { return &Repo{db: db} }

// ── profile ──────────────────────────────────────────────────────────────────

const profileCols = `name, role, headlines, prompt, summary, bio, location, email,
	years_experience, available, focus_areas, facts, socials, avatar_key,
	cv_key, cv_filename, cv_size, cv_sha256, cv_updated_at, updated_at`

func (r *Repo) Profile(ctx context.Context) (Profile, error) {
	var p Profile
	var cv CV
	err := r.db.QueryRow(ctx, `SELECT `+profileCols+` FROM profile WHERE id = 1`).Scan(
		&p.Name, &p.Role, &p.Headlines, &p.Prompt, &p.Summary, &p.Bio, &p.Location, &p.Email,
		&p.YearsExperience, &p.Available, &p.FocusAreas, &p.Facts, &p.Socials, &p.AvatarKey,
		&p.cvKey, &cv.Filename, &cv.Size, &cv.SHA256, &cv.UpdatedAt, &p.UpdatedAt,
	)
	if err != nil {
		return p, err
	}
	p.AvatarURL = storage.URL(p.AvatarKey)
	if p.cvKey != "" {
		cv.URL = "/api/cv"
		p.CV = &cv
	}
	nonNil(&p.Headlines, &p.FocusAreas)
	if p.Facts == nil {
		p.Facts = []Fact{}
	}
	if p.Socials == nil {
		p.Socials = []Social{}
	}
	return p, nil
}

// CVKey is the storage key of the current CV, or "" when none is uploaded.
func (r *Repo) CVKey(ctx context.Context) (key, filename string, err error) {
	err = r.db.QueryRow(ctx, `SELECT cv_key, cv_filename FROM profile WHERE id = 1`).Scan(&key, &filename)
	return
}

// UpdateProfile writes the editable text fields. Files (avatar, CV) have
// their own setters because they are replaced by upload, not by form.
func (r *Repo) UpdateProfile(ctx context.Context, p Profile) error {
	nonNil(&p.Headlines, &p.FocusAreas)
	if p.Facts == nil {
		p.Facts = []Fact{}
	}
	if p.Socials == nil {
		p.Socials = []Social{}
	}
	_, err := r.db.Exec(ctx, `UPDATE profile SET
		name=$1, role=$2, headlines=$3, prompt=$4, summary=$5, bio=$6, location=$7, email=$8,
		years_experience=$9, available=$10, focus_areas=$11, facts=$12, socials=$13,
		avatar_key=$14, updated_at=now()
		WHERE id = 1`,
		p.Name, p.Role, p.Headlines, p.Prompt, p.Summary, p.Bio, p.Location, p.Email,
		p.YearsExperience, p.Available, p.FocusAreas, p.Facts, p.Socials, p.AvatarKey,
	)
	return err
}

func (r *Repo) SetCV(ctx context.Context, key, filename string, size int64, sha string) error {
	_, err := r.db.Exec(ctx, `UPDATE profile SET cv_key=$1, cv_filename=$2, cv_size=$3, cv_sha256=$4,
		cv_updated_at = CASE WHEN $1 = '' THEN NULL ELSE now() END, updated_at=now() WHERE id = 1`,
		key, filename, size, sha)
	return err
}

// ── projects ─────────────────────────────────────────────────────────────────

const projectCols = `id, title, category, platforms, summary, description, tech, image_key,
	live_url, alt_url, repo_url, featured, published, sort_order`

func scanProject(row pgx.Row) (Project, error) {
	var p Project
	err := row.Scan(&p.ID, &p.Title, &p.Category, &p.Platforms, &p.Summary, &p.Description, &p.Tech,
		&p.ImageKey, &p.LiveURL, &p.AltURL, &p.RepoURL, &p.Featured, &p.Published, &p.SortOrder)
	p.ImageURL = storage.URL(p.ImageKey)
	nonNil(&p.Tech)
	return p, err
}

func (r *Repo) Projects(ctx context.Context, publishedOnly bool) ([]Project, error) {
	q := `SELECT ` + projectCols + ` FROM projects`
	if publishedOnly {
		q += ` WHERE published`
	}
	rows, err := r.db.Query(ctx, q+` ORDER BY sort_order, id`)
	if err != nil {
		return nil, err
	}
	return collect(rows, scanProject)
}

func (r *Repo) Project(ctx context.Context, id int64) (Project, error) {
	p, err := scanProject(r.db.QueryRow(ctx, `SELECT `+projectCols+` FROM projects WHERE id = $1`, id))
	return p, notFound(err)
}

func (r *Repo) SaveProject(ctx context.Context, p Project) (Project, error) {
	nonNil(&p.Tech)
	args := []any{p.Title, p.Category, p.Platforms, p.Summary, p.Description, p.Tech, p.ImageKey,
		p.LiveURL, p.AltURL, p.RepoURL, p.Featured, p.Published}
	if p.ID == 0 {
		return scanProject(r.db.QueryRow(ctx, `INSERT INTO projects
			(title, category, platforms, summary, description, tech, image_key, live_url, alt_url, repo_url, featured, published, sort_order)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12, (SELECT COALESCE(MAX(sort_order), 0) + 1 FROM projects))
			RETURNING `+projectCols, args...))
	}
	p, err := scanProject(r.db.QueryRow(ctx, `UPDATE projects SET
		title=$1, category=$2, platforms=$3, summary=$4, description=$5, tech=$6, image_key=$7,
		live_url=$8, alt_url=$9, repo_url=$10, featured=$11, published=$12, updated_at=now()
		WHERE id=$13 RETURNING `+projectCols, append(args, p.ID)...))
	return p, notFound(err)
}

func (r *Repo) DeleteProject(ctx context.Context, id int64) error {
	return r.deleteByID(ctx, "projects", id)
}

// ── experiences ──────────────────────────────────────────────────────────────

const experienceCols = `id, kind, title, organization, period_start, period_end, summary,
	highlights, tech, published, sort_order`

func scanExperience(row pgx.Row) (Experience, error) {
	var e Experience
	err := row.Scan(&e.ID, &e.Kind, &e.Title, &e.Organization, &e.PeriodStart, &e.PeriodEnd,
		&e.Summary, &e.Highlights, &e.Tech, &e.Published, &e.SortOrder)
	nonNil(&e.Highlights, &e.Tech)
	return e, err
}

func (r *Repo) Experiences(ctx context.Context, publishedOnly bool) ([]Experience, error) {
	q := `SELECT ` + experienceCols + ` FROM experiences`
	if publishedOnly {
		q += ` WHERE published`
	}
	rows, err := r.db.Query(ctx, q+` ORDER BY sort_order, id`)
	if err != nil {
		return nil, err
	}
	return collect(rows, scanExperience)
}

func (r *Repo) SaveExperience(ctx context.Context, e Experience) (Experience, error) {
	nonNil(&e.Highlights, &e.Tech)
	if e.Kind != "education" {
		e.Kind = "work"
	}
	args := []any{e.Kind, e.Title, e.Organization, e.PeriodStart, e.PeriodEnd, e.Summary, e.Highlights, e.Tech, e.Published}
	if e.ID == 0 {
		return scanExperience(r.db.QueryRow(ctx, `INSERT INTO experiences
			(kind, title, organization, period_start, period_end, summary, highlights, tech, published, sort_order)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9, (SELECT COALESCE(MAX(sort_order), 0) + 1 FROM experiences))
			RETURNING `+experienceCols, args...))
	}
	e, err := scanExperience(r.db.QueryRow(ctx, `UPDATE experiences SET
		kind=$1, title=$2, organization=$3, period_start=$4, period_end=$5, summary=$6,
		highlights=$7, tech=$8, published=$9, updated_at=now()
		WHERE id=$10 RETURNING `+experienceCols, append(args, e.ID)...))
	return e, notFound(err)
}

func (r *Repo) DeleteExperience(ctx context.Context, id int64) error {
	return r.deleteByID(ctx, "experiences", id)
}

// ── skills ───────────────────────────────────────────────────────────────────

func scanSkill(row pgx.Row) (Skill, error) {
	var s Skill
	err := row.Scan(&s.ID, &s.Name, &s.Category, &s.Level, &s.SortOrder)
	return s, err
}

func (r *Repo) Skills(ctx context.Context) ([]Skill, error) {
	rows, err := r.db.Query(ctx, `SELECT id, name, category, level, sort_order FROM skills ORDER BY sort_order, id`)
	if err != nil {
		return nil, err
	}
	return collect(rows, scanSkill)
}

func (r *Repo) SaveSkill(ctx context.Context, s Skill) (Skill, error) {
	switch s.Level {
	case "Expert", "Advanced", "Intermediate", "Basic":
	default:
		s.Level = "Advanced"
	}
	if s.ID == 0 {
		return scanSkill(r.db.QueryRow(ctx, `INSERT INTO skills (name, category, level, sort_order)
			VALUES ($1, $2, $3, (SELECT COALESCE(MAX(sort_order), 0) + 1 FROM skills))
			RETURNING id, name, category, level, sort_order`, s.Name, s.Category, s.Level))
	}
	s, err := scanSkill(r.db.QueryRow(ctx, `UPDATE skills SET name=$1, category=$2, level=$3 WHERE id=$4
		RETURNING id, name, category, level, sort_order`, s.Name, s.Category, s.Level, s.ID))
	return s, notFound(err)
}

func (r *Repo) DeleteSkill(ctx context.Context, id int64) error {
	return r.deleteByID(ctx, "skills", id)
}

// Reorder sets sort_order to each id's position in ids.
func (r *Repo) Reorder(ctx context.Context, table string, ids []int64) error {
	if !orderable[table] {
		return errors.New("table is not orderable")
	}
	return pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		for i, id := range ids {
			if _, err := tx.Exec(ctx, `UPDATE `+table+` SET sort_order = $1 WHERE id = $2`, i+1, id); err != nil {
				return err
			}
		}
		return nil
	})
}

var orderable = map[string]bool{"projects": true, "experiences": true, "skills": true}

// ── messages ─────────────────────────────────────────────────────────────────

func (r *Repo) CreateMessage(ctx context.Context, name, email, body, ip, ua string) error {
	_, err := r.db.Exec(ctx, `INSERT INTO messages (name, email, body, ip, user_agent) VALUES ($1,$2,$3,$4,$5)`,
		name, email, body, ip, ua)
	return err
}

func (r *Repo) Messages(ctx context.Context) ([]Message, error) {
	rows, err := r.db.Query(ctx, `SELECT id, name, email, body, read, created_at FROM messages ORDER BY created_at DESC LIMIT 500`)
	if err != nil {
		return nil, err
	}
	return collect(rows, func(row pgx.Row) (Message, error) {
		var m Message
		err := row.Scan(&m.ID, &m.Name, &m.Email, &m.Body, &m.Read, &m.CreatedAt)
		return m, err
	})
}

func (r *Repo) MarkMessage(ctx context.Context, id int64, read bool) error {
	tag, err := r.db.Exec(ctx, `UPDATE messages SET read = $1 WHERE id = $2`, read, id)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

func (r *Repo) DeleteMessage(ctx context.Context, id int64) error {
	return r.deleteByID(ctx, "messages", id)
}

// ── counters & stats ─────────────────────────────────────────────────────────

func (r *Repo) Increment(ctx context.Context, name string) {
	_, _ = r.db.Exec(ctx, `INSERT INTO counters (name, value) VALUES ($1, 1)
		ON CONFLICT (name) DO UPDATE SET value = counters.value + 1`, name)
}

func (r *Repo) Stats(ctx context.Context) (Stats, error) {
	var s Stats
	err := r.db.QueryRow(ctx, `SELECT
		COALESCE((SELECT value FROM counters WHERE name = 'cv_downloads'), 0),
		COALESCE((SELECT value FROM counters WHERE name = 'site_views'), 0),
		(SELECT COUNT(*) FROM projects WHERE published),
		(SELECT COUNT(*) FROM messages WHERE NOT read),
		(SELECT COUNT(*) FROM messages)`).Scan(&s.CVDownloads, &s.SiteViews, &s.Projects, &s.UnreadMessages, &s.Messages)
	return s, err
}

// Site assembles the public payload.
func (r *Repo) Site(ctx context.Context) (Site, error) {
	var s Site
	var err error
	if s.Profile, err = r.Profile(ctx); err != nil {
		return s, err
	}
	if s.Projects, err = r.Projects(ctx, true); err != nil {
		return s, err
	}
	if s.Experiences, err = r.Experiences(ctx, true); err != nil {
		return s, err
	}
	s.Skills, err = r.Skills(ctx)
	return s, err
}

// ── helpers ──────────────────────────────────────────────────────────────────

func (r *Repo) deleteByID(ctx context.Context, table string, id int64) error {
	tag, err := r.db.Exec(ctx, `DELETE FROM `+table+` WHERE id = $1`, id)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

func collect[T any](rows pgx.Rows, scan func(pgx.Row) (T, error)) ([]T, error) {
	defer rows.Close()
	out := []T{}
	for rows.Next() {
		v, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func notFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

func nonNil(slices ...*[]string) {
	for _, s := range slices {
		if *s == nil {
			*s = []string{}
		}
		clean := (*s)[:0]
		for _, v := range *s {
			if v = strings.TrimSpace(v); v != "" {
				clean = append(clean, v)
			}
		}
		*s = clean
	}
}
