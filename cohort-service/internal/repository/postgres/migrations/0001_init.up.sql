CREATE EXTENSION IF NOT EXISTS pgcrypto;

DO $$ BEGIN
    CREATE TYPE cohort_type AS ENUM ('static', 'sql');
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

DO $$ BEGIN
    CREATE TYPE refresh_status AS ENUM ('pending', 'running', 'succeeded', 'failed');
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

CREATE TABLE IF NOT EXISTS cohorts (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name              TEXT NOT NULL UNIQUE,
    description       TEXT NOT NULL DEFAULT '',
    type              cohort_type NOT NULL,
    sql_query         TEXT,
    static_users      TEXT[],
    size              INT NOT NULL DEFAULT 0,
    last_refreshed_at TIMESTAMPTZ,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK ((type = 'sql' AND sql_query IS NOT NULL) OR
           (type = 'static' AND static_users IS NOT NULL))
);

CREATE TABLE IF NOT EXISTS cohort_refresh_runs (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    cohort_id     UUID NOT NULL REFERENCES cohorts(id) ON DELETE CASCADE,
    status        refresh_status NOT NULL,
    added_count   INT NOT NULL DEFAULT 0,
    removed_count INT NOT NULL DEFAULT 0,
    error         TEXT NOT NULL DEFAULT '',
    started_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    finished_at   TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_refresh_runs_cohort_started
    ON cohort_refresh_runs (cohort_id, started_at DESC);
