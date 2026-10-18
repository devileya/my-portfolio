-- One row per site view and CV download, for the overview's visitor
-- analytics. The counters table keeps the all-time totals.
CREATE TABLE visits (
    id            BIGSERIAL PRIMARY KEY,
    kind          TEXT NOT NULL CHECK (kind IN ('view', 'cv')),
    at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- The visitor's address as Cloudflare saw it, with the host part masked
    -- (/24 for IPv4, /48 for IPv6): enough for network and place, not a person.
    ip            TEXT NOT NULL DEFAULT '',
    country_code  TEXT NOT NULL DEFAULT '',
    region        TEXT NOT NULL DEFAULT '',
    city          TEXT NOT NULL DEFAULT '',
    referrer      TEXT NOT NULL DEFAULT '',
    device        TEXT NOT NULL DEFAULT '',
    browser       TEXT NOT NULL DEFAULT '',
    os            TEXT NOT NULL DEFAULT '',
    user_agent    TEXT NOT NULL DEFAULT '',
    is_bot        BOOLEAN NOT NULL DEFAULT FALSE,
    -- Signed in to the dashboard: your own visits, left out of the numbers.
    is_owner      BOOLEAN NOT NULL DEFAULT FALSE
);
CREATE INDEX visits_at_idx ON visits (at DESC);
CREATE INDEX visits_kind_at_idx ON visits (kind, at DESC);
