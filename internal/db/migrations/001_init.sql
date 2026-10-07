CREATE TABLE admin_users (
    id            BIGSERIAL PRIMARY KEY,
    email         TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Single-row table: the person this site is about.
CREATE TABLE profile (
    id                INT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    name              TEXT NOT NULL DEFAULT '',
    role              TEXT NOT NULL DEFAULT '',
    -- The hero "generates" one of these. Index 0 is the greedy (T=0) answer.
    headlines         TEXT[] NOT NULL DEFAULT '{}',
    prompt            TEXT NOT NULL DEFAULT '',
    summary           TEXT NOT NULL DEFAULT '',
    bio               TEXT NOT NULL DEFAULT '',
    location          TEXT NOT NULL DEFAULT '',
    email             TEXT NOT NULL DEFAULT '',
    years_experience  INT NOT NULL DEFAULT 0,
    available         BOOLEAN NOT NULL DEFAULT TRUE,
    focus_areas       TEXT[] NOT NULL DEFAULT '{}',
    -- [{label, value, url?}] rendered as the model card.
    facts             JSONB NOT NULL DEFAULT '[]',
    -- [{label, url}]
    socials           JSONB NOT NULL DEFAULT '[]',
    avatar_key        TEXT NOT NULL DEFAULT '',
    cv_key            TEXT NOT NULL DEFAULT '',
    cv_filename       TEXT NOT NULL DEFAULT '',
    cv_size           BIGINT NOT NULL DEFAULT 0,
    cv_sha256         TEXT NOT NULL DEFAULT '',
    cv_updated_at     TIMESTAMPTZ,
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE projects (
    id           BIGSERIAL PRIMARY KEY,
    title        TEXT NOT NULL,
    category     TEXT NOT NULL DEFAULT 'web',
    platforms    TEXT NOT NULL DEFAULT '',
    summary      TEXT NOT NULL DEFAULT '',
    description  TEXT NOT NULL DEFAULT '',
    tech         TEXT[] NOT NULL DEFAULT '{}',
    image_key    TEXT NOT NULL DEFAULT '',
    live_url     TEXT NOT NULL DEFAULT '',
    alt_url      TEXT NOT NULL DEFAULT '',
    repo_url     TEXT NOT NULL DEFAULT '',
    featured     BOOLEAN NOT NULL DEFAULT FALSE,
    published    BOOLEAN NOT NULL DEFAULT TRUE,
    sort_order   INT NOT NULL DEFAULT 0,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Work history and education share one timeline.
CREATE TABLE experiences (
    id            BIGSERIAL PRIMARY KEY,
    kind          TEXT NOT NULL DEFAULT 'work' CHECK (kind IN ('work', 'education')),
    title         TEXT NOT NULL,
    organization  TEXT NOT NULL DEFAULT '',
    period_start  TEXT NOT NULL DEFAULT '',
    period_end    TEXT NOT NULL DEFAULT '',
    summary       TEXT NOT NULL DEFAULT '',
    highlights    TEXT[] NOT NULL DEFAULT '{}',
    tech          TEXT[] NOT NULL DEFAULT '{}',
    published     BOOLEAN NOT NULL DEFAULT TRUE,
    sort_order    INT NOT NULL DEFAULT 0,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE skills (
    id          BIGSERIAL PRIMARY KEY,
    name        TEXT NOT NULL,
    category    TEXT NOT NULL DEFAULT '',
    level       TEXT NOT NULL DEFAULT 'Advanced' CHECK (level IN ('Expert', 'Advanced', 'Intermediate', 'Basic')),
    sort_order  INT NOT NULL DEFAULT 0
);

CREATE TABLE messages (
    id          BIGSERIAL PRIMARY KEY,
    name        TEXT NOT NULL,
    email       TEXT NOT NULL,
    body        TEXT NOT NULL,
    ip          TEXT NOT NULL DEFAULT '',
    user_agent  TEXT NOT NULL DEFAULT '',
    read        BOOLEAN NOT NULL DEFAULT FALSE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX messages_created_at_idx ON messages (created_at DESC);

CREATE TABLE counters (
    name   TEXT PRIMARY KEY,
    value  BIGINT NOT NULL DEFAULT 0
);
INSERT INTO counters (name) VALUES ('cv_downloads'), ('site_views');

INSERT INTO profile (id) VALUES (1);
