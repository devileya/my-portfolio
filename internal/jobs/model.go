// Package jobs is the Job Hunter: saved searches run on a schedule against
// job portals, an LLM scores each listing against the portfolio's profile,
// good matches wait in a review queue (or are approved automatically), and a
// browser worker fills in and submits the applications.
package jobs

import (
	"encoding/json"
	"time"
)

var Portals = []string{"linkedin", "indeed", "glints", "kalibrr"}

var PortalLabel = map[string]string{
	"linkedin": "LinkedIn",
	"indeed":   "Indeed",
	"glints":   "Glints",
	"kalibrr":  "Kalibrr",
}

type Settings struct {
	Paused           bool              `json:"paused"`
	DryRun           bool              `json:"dryRun"`
	Timezone         string            `json:"timezone"`
	ApplyStartHour   int               `json:"applyStartHour"`
	ApplyEndHour     int               `json:"applyEndHour"`
	MinGapMinutes    int               `json:"minGapMinutes"`
	MaxGapMinutes    int               `json:"maxGapMinutes"`
	PortalCaps       map[string]int    `json:"portalCaps"`
	ScoreProviderID  *int64            `json:"scoreProviderId"`
	ApplyProviderID  *int64            `json:"applyProviderId"`
	Applicant        map[string]string `json:"applicant"`
	CoverLetterStyle string            `json:"coverLetterStyle"`
	NotifyMatches    bool              `json:"notifyMatches"`
	NotifyApplied    bool              `json:"notifyApplied"`
	NotifyDigest     bool              `json:"notifyDigest"`
	DigestHour       int               `json:"digestHour"`
	// Telegram is read-only here; it's saved through SaveTelegram.
	Telegram     TelegramInfo `json:"telegram"`
	WorkerName   string       `json:"workerName"`
	WorkerState  string       `json:"workerState"`
	WorkerSeenAt *time.Time   `json:"workerSeenAt"`
	UpdatedAt    time.Time    `json:"updatedAt"`
}

func (s Settings) Location() *time.Location {
	if loc, err := time.LoadLocation(s.Timezone); err == nil {
		return loc
	}
	return time.UTC
}

// TelegramInfo describes the configured bot without exposing the token.
type TelegramInfo struct {
	HasToken  bool   `json:"hasToken"`
	TokenHint string `json:"tokenHint"`
	BotName   string `json:"botName"`
	ChatID    int64  `json:"chatId"`
	ChatName  string `json:"chatName"`
	Poll      bool   `json:"poll"`
}

// ApplicantFields are the keys the dashboard offers for Settings.Applicant.
// Anything else the user adds is passed to the model as-is.
var ApplicantFields = []string{
	"firstName", "lastName", "phone", "city", "country", "linkedinUrl", "githubUrl", "portfolioUrl",
	"currentSalary", "salaryFromRange", "noticePeriod", "availableFrom",
	"workAuthorization", "willingToRelocate", "gender", "dateOfBirth", "highestEducation", "languages",
}

type Provider struct {
	ID        int64     `json:"id"`
	Label     string    `json:"label"`
	Kind      string    `json:"kind"`
	BaseURL   string    `json:"baseUrl"`
	Model     string    `json:"model"`
	KeyHint   string    `json:"keyHint"`
	HasKey    bool      `json:"hasKey"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`

	// APIKey is write-only: accepted from the dashboard, never returned.
	APIKey string `json:"apiKey,omitempty"`
}

type Account struct {
	Portal       string     `json:"portal"`
	Label        string     `json:"label"`
	Enabled      bool       `json:"enabled"`
	Status       string     `json:"status"`
	StatusNote   string     `json:"statusNote"`
	CookiesCount int        `json:"cookiesCount"`
	CheckedAt    *time.Time `json:"checkedAt"`
	UpdatedAt    time.Time  `json:"updatedAt"`
}

type Search struct {
	ID               int64      `json:"id"`
	Name             string     `json:"name"`
	Enabled          bool       `json:"enabled"`
	Portals          []string   `json:"portals"`
	Keywords         []string   `json:"keywords"`
	ExcludeKeywords  []string   `json:"excludeKeywords"`
	ExcludeCompanies []string   `json:"excludeCompanies"`
	Locations        []string   `json:"locations"`
	WorkModes        []string   `json:"workModes"`
	JobTypes         []string   `json:"jobTypes"`
	Seniority        []string   `json:"seniority"`
	MinSalary        int64      `json:"minSalary"`
	PostedWithinDays int        `json:"postedWithinDays"`
	EasyApplyOnly    bool       `json:"easyApplyOnly"`
	Schedule         string     `json:"schedule"`
	MaxResults       int        `json:"maxResults"`
	MinScore         int        `json:"minScore"`
	AutoApply        bool       `json:"autoApply"`
	AutoMinScore     int        `json:"autoMinScore"`
	DailyCap         int        `json:"dailyCap"`
	Notes            string     `json:"notes"`
	LastRunAt        *time.Time `json:"lastRunAt"`
	NextRunAt        *time.Time `json:"nextRunAt"`
	CreatedAt        time.Time  `json:"createdAt"`
	UpdatedAt        time.Time  `json:"updatedAt"`
}

func (s *Search) SetID(id int64) { s.ID = id }

var (
	WorkModes = []string{"remote", "hybrid", "onsite"}
	JobTypes  = []string{"full_time", "contract", "part_time", "internship", "freelance"}
	Seniority = []string{"internship", "entry", "associate", "mid", "senior", "lead", "director"}
)

type ScoreDetails struct {
	Verdict  string   `json:"verdict,omitempty"`
	Matched  []string `json:"matched,omitempty"`
	Missing  []string `json:"missing,omitempty"`
	Concerns []string `json:"concerns,omitempty"`
}

type Listing struct {
	ID         int64  `json:"id"`
	Portal     string `json:"portal"`
	ExternalID string `json:"externalId"`
	SearchID   *int64 `json:"searchId"`
	SearchName string `json:"searchName"`
	URL        string `json:"url"`
	Title      string `json:"title"`
	Company    string `json:"company"`
	Location   string `json:"location"`
	WorkMode   string `json:"workMode"`
	JobType    string `json:"jobType"`
	Salary     string `json:"salary"`
	// The posting's range as numbers, when it states one.
	SalaryMin      *int64       `json:"salaryMin"`
	SalaryMax      *int64       `json:"salaryMax"`
	SalaryCurrency string       `json:"salaryCurrency"`
	SalaryPeriod   string       `json:"salaryPeriod"`
	Description    string       `json:"description,omitempty"`
	EasyApply      *bool        `json:"easyApply"`
	PostedAt       *time.Time   `json:"postedAt"`
	Score          *int         `json:"score"`
	ScoreReason    string       `json:"scoreReason"`
	ScoreDetails   ScoreDetails `json:"scoreDetails"`
	Status         string       `json:"status"`
	StatusNote     string       `json:"statusNote"`
	Attempts       int          `json:"attempts"`
	ApprovedAt     *time.Time   `json:"approvedAt"`
	AppliedAt      *time.Time   `json:"appliedAt"`
	FoundAt        time.Time    `json:"foundAt"`
	UpdatedAt      time.Time    `json:"updatedAt"`
}

// Listing statuses.
const (
	StatusNew        = "new"
	StatusFiltered   = "filtered"
	StatusQueued     = "queued"
	StatusSkipped    = "skipped"
	StatusApproved   = "approved"
	StatusApplying   = "applying"
	StatusApplied    = "applied"
	StatusNeedsInput = "needs_input"
	StatusExternal   = "external"
	StatusFailed     = "failed"
	StatusDryRun     = "dry_run"
)

type Answer struct {
	Question string `json:"question"`
	Answer   string `json:"answer"`
	Source   string `json:"source"`
}

type Question struct {
	Key      string   `json:"key"`
	Question string   `json:"question"`
	Kind     string   `json:"kind"`
	Options  []string `json:"options,omitempty"`
}

type Application struct {
	ID               int64      `json:"id"`
	ListingID        int64      `json:"listingId"`
	Listing          *Listing   `json:"listing,omitempty"`
	Status           string     `json:"status"`
	Answers          []Answer   `json:"answers"`
	PendingQuestions []Question `json:"pendingQuestions"`
	CoverLetter      string     `json:"coverLetter"`
	ScreenshotKey    string     `json:"screenshotKey"`
	ScreenshotURL    string     `json:"screenshotUrl"`
	Error            string     `json:"error"`
	Steps            int        `json:"steps"`
	StartedAt        time.Time  `json:"startedAt"`
	FinishedAt       *time.Time `json:"finishedAt"`
}

type SavedAnswer struct {
	ID        int64     `json:"id"`
	Key       string    `json:"key"`
	Question  string    `json:"question"`
	Answer    string    `json:"answer"`
	Source    string    `json:"source"`
	Approved  bool      `json:"approved"`
	UsedCount int       `json:"usedCount"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func (a *SavedAnswer) SetID(id int64) { a.ID = id }

type Task struct {
	ID         int64           `json:"id"`
	Kind       string          `json:"kind"`
	SearchID   *int64          `json:"searchId"`
	SearchName string          `json:"searchName"`
	Portal     string          `json:"portal"`
	Status     string          `json:"status"`
	Trigger    string          `json:"trigger"`
	RunAfter   time.Time       `json:"runAfter"`
	LockedBy   string          `json:"lockedBy"`
	StartedAt  *time.Time      `json:"startedAt"`
	FinishedAt *time.Time      `json:"finishedAt"`
	Result     json.RawMessage `json:"result"`
	Log        string          `json:"log"`
	Error      string          `json:"error"`
	CreatedAt  time.Time       `json:"createdAt"`
}

type TaskResult struct {
	Found    int `json:"found"`
	New      int `json:"new"`
	Filtered int `json:"filtered"`
	Scored   int `json:"scored"`
	Queued   int `json:"queued"`
	Approved int `json:"approved"`
}

type Overview struct {
	Settings      Settings       `json:"settings"`
	Accounts      []Account      `json:"accounts"`
	ByStatus      map[string]int `json:"byStatus"`
	AppliedToday  map[string]int `json:"appliedToday"`
	AppliedWeek   int            `json:"appliedWeek"`
	Searches      int            `json:"searches"`
	ActiveSearch  int            `json:"activeSearches"`
	PendingTasks  int            `json:"pendingTasks"`
	RecentTasks   []Task         `json:"recentTasks"`
	TelegramReady bool           `json:"telegramReady"`
	SecretsReady  bool           `json:"secretsReady"`
	WorkerMode    string         `json:"workerMode"`
	Providers     int            `json:"providers"`
	TopQueued     []Listing      `json:"topQueued"`
}
