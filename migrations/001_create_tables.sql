-- 001_create_tables.sql
-- Kestrel webhook delivery engine — initial schema

CREATE TABLE IF NOT EXISTS events (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    type            TEXT NOT NULL,
    payload         JSONB NOT NULL,
    idempotency_key TEXT UNIQUE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_events_type ON events(type);
CREATE INDEX IF NOT EXISTS idx_events_created_at ON events(created_at DESC);

CREATE TABLE IF NOT EXISTS subscriptions (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    endpoint_url  TEXT NOT NULL,
    secret        TEXT NOT NULL,
    event_types   TEXT[] NOT NULL,
    is_active     BOOLEAN NOT NULL DEFAULT true,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS delivery_jobs (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_id        UUID NOT NULL REFERENCES events(id),
    subscription_id UUID NOT NULL REFERENCES subscriptions(id),
    status          TEXT NOT NULL DEFAULT 'pending',
    attempt_count   INT NOT NULL DEFAULT 0,
    max_attempts    INT NOT NULL DEFAULT 5,
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_error      TEXT,
    last_status_code INT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    delivered_at    TIMESTAMPTZ
);

-- CRITICAL: Partial index on pending jobs only. This is the interview talking point.
-- Full index would include delivered/dead rows (majority). Partial stays small and hot.
CREATE INDEX IF NOT EXISTS idx_delivery_jobs_pending
    ON delivery_jobs (next_attempt_at ASC)
    WHERE status IN ('pending');

CREATE INDEX IF NOT EXISTS idx_delivery_jobs_event
    ON delivery_jobs (event_id);
