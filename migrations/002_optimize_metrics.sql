-- 002_optimize_metrics.sql
-- Optimizes counts using triggers so we don't need to run COUNT(*) on millions of rows.

CREATE TABLE IF NOT EXISTS system_stats (
    key TEXT PRIMARY KEY,
    value BIGINT NOT NULL DEFAULT 0
);

DO $$
BEGIN
    -- Prevent deadlocks with concurrent worker queries (e.g. DequeueBatch) by locking both tables atomically
    LOCK TABLE events, delivery_jobs IN SHARE ROW EXCLUSIVE MODE;

    -- Only run these inserts/updates if the table hasn't been migrated to multi-tenant yet.
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='system_stats' AND column_name='user_id') THEN
        INSERT INTO system_stats (key, value) VALUES ('total_events', 0) ON CONFLICT DO NOTHING;
        INSERT INTO system_stats (key, value) VALUES ('delivery_pending', 0) ON CONFLICT DO NOTHING;
        INSERT INTO system_stats (key, value) VALUES ('delivery_in_flight', 0) ON CONFLICT DO NOTHING;
        INSERT INTO system_stats (key, value) VALUES ('delivery_delivered', 0) ON CONFLICT DO NOTHING;
        INSERT INTO system_stats (key, value) VALUES ('delivery_failed', 0) ON CONFLICT DO NOTHING;
        INSERT INTO system_stats (key, value) VALUES ('delivery_dead', 0) ON CONFLICT DO NOTHING;

        UPDATE system_stats SET value = (SELECT COUNT(*) FROM events) WHERE key = 'total_events';
        UPDATE system_stats SET value = (SELECT COUNT(*) FROM delivery_jobs WHERE status = 'pending') WHERE key = 'delivery_pending';
        UPDATE system_stats SET value = (SELECT COUNT(*) FROM delivery_jobs WHERE status = 'in_flight') WHERE key = 'delivery_in_flight';
        UPDATE system_stats SET value = (SELECT COUNT(*) FROM delivery_jobs WHERE status = 'delivered') WHERE key = 'delivery_delivered';
        UPDATE system_stats SET value = (SELECT COUNT(*) FROM delivery_jobs WHERE status = 'failed') WHERE key = 'delivery_failed';
        UPDATE system_stats SET value = (SELECT COUNT(*) FROM delivery_jobs WHERE status = 'dead') WHERE key = 'delivery_dead';
    END IF;
END $$;

-- Function for events table
CREATE OR REPLACE FUNCTION trg_events_stats() RETURNS TRIGGER AS $$
BEGIN
    IF TG_OP = 'INSERT' THEN
        UPDATE system_stats SET value = value + 1 WHERE key = 'total_events';
    ELSIF TG_OP = 'DELETE' THEN
        UPDATE system_stats SET value = value - 1 WHERE key = 'total_events';
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_events_stats_trigger ON events;
CREATE TRIGGER trg_events_stats_trigger
AFTER INSERT OR DELETE ON events
FOR EACH ROW EXECUTE FUNCTION trg_events_stats();

-- Function for delivery_jobs table
CREATE OR REPLACE FUNCTION trg_delivery_jobs_stats() RETURNS TRIGGER AS $$
BEGIN
    IF TG_OP = 'INSERT' THEN
        UPDATE system_stats SET value = value + 1 WHERE key = 'delivery_' || NEW.status;
    ELSIF TG_OP = 'UPDATE' THEN
        IF OLD.status != NEW.status THEN
            UPDATE system_stats SET value = value - 1 WHERE key = 'delivery_' || OLD.status;
            UPDATE system_stats SET value = value + 1 WHERE key = 'delivery_' || NEW.status;
        END IF;
    ELSIF TG_OP = 'DELETE' THEN
        UPDATE system_stats SET value = value - 1 WHERE key = 'delivery_' || OLD.status;
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_delivery_jobs_stats_trigger ON delivery_jobs;
CREATE TRIGGER trg_delivery_jobs_stats_trigger
AFTER INSERT OR UPDATE OR DELETE ON delivery_jobs
FOR EACH ROW EXECUTE FUNCTION trg_delivery_jobs_stats();
