-- Job Hunter: saved searches run on a schedule, listings are scored by an LLM
-- and queued for review, and approved listings are applied to by a browser
-- worker. Secrets (API keys, portal cookies) are sealed with JOBS_SECRET_KEY.

-- Single-row settings for the whole module.
CREATE TABLE job_settings (
    id                  INT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    paused              BOOLEAN NOT NULL DEFAULT FALSE,
    -- Fill forms but never press the final submit button.
    dry_run             BOOLEAN NOT NULL DEFAULT TRUE,
    timezone            TEXT NOT NULL DEFAULT 'Asia/Jakarta',
    -- Applications are only sent between these local hours.
    apply_start_hour    INT NOT NULL DEFAULT 8 CHECK (apply_start_hour BETWEEN 0 AND 23),
    apply_end_hour      INT NOT NULL DEFAULT 22 CHECK (apply_end_hour BETWEEN 1 AND 24),
    -- Random gap between two applications on the same portal.
    min_gap_minutes     INT NOT NULL DEFAULT 4,
    max_gap_minutes     INT NOT NULL DEFAULT 12,
    -- {"linkedin": 15, ...}: applications per portal per local day.
    portal_caps         JSONB NOT NULL DEFAULT '{"linkedin": 15, "indeed": 20, "glints": 25, "kalibrr": 25}',
    score_provider_id   BIGINT,
    apply_provider_id   BIGINT,
    -- Free-form facts for application forms: phone, expected salary, notice period…
    applicant           JSONB NOT NULL DEFAULT '{}',
    cover_letter_style  TEXT NOT NULL DEFAULT '',
    notify_matches      BOOLEAN NOT NULL DEFAULT TRUE,
    notify_applied      BOOLEAN NOT NULL DEFAULT TRUE,
    notify_digest       BOOLEAN NOT NULL DEFAULT TRUE,
    digest_hour         INT NOT NULL DEFAULT 21 CHECK (digest_hour BETWEEN 0 AND 23),
    -- Telegram bot, configured from the dashboard. The token is sealed.
    telegram_token_sealed BYTEA,
    telegram_token_hint   TEXT NOT NULL DEFAULT '',
    telegram_bot_name     TEXT NOT NULL DEFAULT '',
    telegram_chat_id      BIGINT NOT NULL DEFAULT 0,
    telegram_chat_name    TEXT NOT NULL DEFAULT '',
    -- Receive button presses and replies. Only one process may poll a bot.
    telegram_poll         BOOLEAN NOT NULL DEFAULT TRUE,
    last_digest_on      DATE,
    worker_name         TEXT NOT NULL DEFAULT '',
    worker_state        TEXT NOT NULL DEFAULT '',
    worker_seen_at      TIMESTAMPTZ,
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);
INSERT INTO job_settings (id) VALUES (1);

CREATE TABLE job_ai_providers (
    id          BIGSERIAL PRIMARY KEY,
    label       TEXT NOT NULL,
    -- 'anthropic' uses the Messages API; 'openai' is any /chat/completions API
    -- (OpenAI, Vikey, OpenRouter, Gemini's OpenAI endpoint, DeepSeek…).
    kind        TEXT NOT NULL CHECK (kind IN ('anthropic', 'openai')),
    base_url    TEXT NOT NULL DEFAULT '',
    model       TEXT NOT NULL,
    key_sealed  BYTEA,
    key_hint    TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE job_accounts (
    portal          TEXT PRIMARY KEY CHECK (portal IN ('linkedin', 'indeed', 'glints', 'kalibrr')),
    enabled         BOOLEAN NOT NULL DEFAULT TRUE,
    -- disconnected | connected | expired | challenge | error
    status          TEXT NOT NULL DEFAULT 'disconnected',
    status_note     TEXT NOT NULL DEFAULT '',
    cookies_sealed  BYTEA,
    cookies_count   INT NOT NULL DEFAULT 0,
    checked_at      TIMESTAMPTZ,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
INSERT INTO job_accounts (portal) VALUES ('linkedin'), ('indeed'), ('glints'), ('kalibrr');

CREATE TABLE job_searches (
    id                  BIGSERIAL PRIMARY KEY,
    name                TEXT NOT NULL,
    enabled             BOOLEAN NOT NULL DEFAULT TRUE,
    portals             TEXT[] NOT NULL DEFAULT '{linkedin,indeed,glints,kalibrr}',
    keywords            TEXT[] NOT NULL DEFAULT '{}',
    exclude_keywords    TEXT[] NOT NULL DEFAULT '{}',
    exclude_companies   TEXT[] NOT NULL DEFAULT '{}',
    locations           TEXT[] NOT NULL DEFAULT '{}',
    -- any of: remote, hybrid, onsite (empty = all)
    work_modes          TEXT[] NOT NULL DEFAULT '{}',
    -- any of: full_time, contract, part_time, internship, freelance (empty = all)
    job_types           TEXT[] NOT NULL DEFAULT '{}',
    -- any of: internship, entry, associate, mid, senior, lead, director (empty = all)
    seniority           TEXT[] NOT NULL DEFAULT '{}',
    min_salary          BIGINT NOT NULL DEFAULT 0,
    posted_within_days  INT NOT NULL DEFAULT 7,
    easy_apply_only     BOOLEAN NOT NULL DEFAULT TRUE,
    -- Standard 5-field cron, evaluated in job_settings.timezone.
    schedule            TEXT NOT NULL DEFAULT '0 9,13,17 * * *',
    max_results         INT NOT NULL DEFAULT 25,
    -- Listings scoring at least min_score enter the review queue…
    min_score           INT NOT NULL DEFAULT 60,
    -- …and with auto_apply on, those at auto_min_score or above are approved
    -- without review.
    auto_apply          BOOLEAN NOT NULL DEFAULT FALSE,
    auto_min_score      INT NOT NULL DEFAULT 85,
    daily_cap           INT NOT NULL DEFAULT 20,
    notes               TEXT NOT NULL DEFAULT '',
    last_run_at         TIMESTAMPTZ,
    next_run_at         TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE job_listings (
    id              BIGSERIAL PRIMARY KEY,
    portal          TEXT NOT NULL,
    external_id     TEXT NOT NULL,
    search_id       BIGINT REFERENCES job_searches(id) ON DELETE SET NULL,
    url             TEXT NOT NULL,
    title           TEXT NOT NULL,
    company         TEXT NOT NULL DEFAULT '',
    location        TEXT NOT NULL DEFAULT '',
    work_mode       TEXT NOT NULL DEFAULT '',
    job_type        TEXT NOT NULL DEFAULT '',
    salary          TEXT NOT NULL DEFAULT '',
    description     TEXT NOT NULL DEFAULT '',
    easy_apply      BOOLEAN,
    posted_at       TIMESTAMPTZ,
    -- lower(company)|normalized title: the same job seen on two portals.
    fingerprint     TEXT NOT NULL DEFAULT '',
    score           INT,
    score_reason    TEXT NOT NULL DEFAULT '',
    score_details   JSONB NOT NULL DEFAULT '{}',
    -- new → scored → queued | filtered; queued → approved | skipped;
    -- approved → applying → applied | needs_input | external | failed | dry_run
    status          TEXT NOT NULL DEFAULT 'new',
    status_note     TEXT NOT NULL DEFAULT '',
    attempts        INT NOT NULL DEFAULT 0,
    approved_at     TIMESTAMPTZ,
    applied_at      TIMESTAMPTZ,
    found_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (portal, external_id)
);
CREATE INDEX job_listings_status_idx ON job_listings (status, score DESC);
CREATE INDEX job_listings_fingerprint_idx ON job_listings (fingerprint);
CREATE INDEX job_listings_applied_idx ON job_listings (portal, applied_at);

-- One row per apply attempt.
CREATE TABLE job_applications (
    id                BIGSERIAL PRIMARY KEY,
    listing_id        BIGINT NOT NULL REFERENCES job_listings(id) ON DELETE CASCADE,
    -- running | submitted | dry_run | needs_input | external | failed
    status            TEXT NOT NULL DEFAULT 'running',
    -- [{question, answer, source}]
    answers           JSONB NOT NULL DEFAULT '[]',
    -- [{key, question, kind, options}]: what the worker could not answer.
    pending_questions JSONB NOT NULL DEFAULT '[]',
    cover_letter      TEXT NOT NULL DEFAULT '',
    screenshot_key    TEXT NOT NULL DEFAULT '',
    error             TEXT NOT NULL DEFAULT '',
    steps             INT NOT NULL DEFAULT 0,
    started_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    finished_at       TIMESTAMPTZ
);
CREATE INDEX job_applications_listing_idx ON job_applications (listing_id, started_at DESC);

-- Answers to screening questions, reused across applications.
CREATE TABLE job_answers (
    id          BIGSERIAL PRIMARY KEY,
    key         TEXT NOT NULL UNIQUE,
    question    TEXT NOT NULL,
    answer      TEXT NOT NULL DEFAULT '',
    -- manual answers are trusted; ai ones are suggestions until approved.
    source      TEXT NOT NULL DEFAULT 'manual' CHECK (source IN ('manual', 'ai')),
    approved    BOOLEAN NOT NULL DEFAULT TRUE,
    used_count  INT NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Work for the browser worker. Applying is driven by approved listings, not
-- tasks; tasks are searches, logins and session checks.
CREATE TABLE job_tasks (
    id           BIGSERIAL PRIMARY KEY,
    kind         TEXT NOT NULL CHECK (kind IN ('search', 'login', 'check')),
    search_id    BIGINT REFERENCES job_searches(id) ON DELETE CASCADE,
    portal       TEXT NOT NULL DEFAULT '',
    -- pending | running | done | failed | cancelled
    status       TEXT NOT NULL DEFAULT 'pending',
    trigger      TEXT NOT NULL DEFAULT 'schedule',
    run_after    TIMESTAMPTZ NOT NULL DEFAULT now(),
    locked_by    TEXT NOT NULL DEFAULT '',
    started_at   TIMESTAMPTZ,
    finished_at  TIMESTAMPTZ,
    -- {found, new, queued, approved, filtered}
    result       JSONB NOT NULL DEFAULT '{}',
    log          TEXT NOT NULL DEFAULT '',
    error        TEXT NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX job_tasks_pending_idx ON job_tasks (status, run_after);

-- Telegram messages that expect a reply or a button press.
CREATE TABLE job_telegram_refs (
    message_id   BIGINT PRIMARY KEY,
    kind         TEXT NOT NULL,
    listing_id   BIGINT REFERENCES job_listings(id) ON DELETE CASCADE,
    question_key TEXT NOT NULL DEFAULT '',
    question     TEXT NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
