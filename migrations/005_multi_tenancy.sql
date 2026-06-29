-- 005_multi_tenancy.sql

-- 1. Create api_keys table
CREATE TABLE IF NOT EXISTS api_keys (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    key        TEXT UNIQUE NOT NULL,
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 2. Add user_id to core tables (allowing NULL initially)
ALTER TABLE subscriptions ADD COLUMN IF NOT EXISTS user_id UUID REFERENCES users(id) ON DELETE CASCADE;
ALTER TABLE events ADD COLUMN IF NOT EXISTS user_id UUID REFERENCES users(id) ON DELETE CASCADE;
ALTER TABLE delivery_jobs ADD COLUMN IF NOT EXISTS user_id UUID REFERENCES users(id) ON DELETE CASCADE;

-- 3. Backfill data to the admin user
DO $$
DECLARE
    admin_id UUID;
BEGIN
    SELECT id INTO admin_id FROM users WHERE email = 'admin@kestrel.local' LIMIT 1;
    IF admin_id IS NOT NULL THEN
        UPDATE subscriptions SET user_id = admin_id WHERE user_id IS NULL;
        UPDATE events SET user_id = admin_id WHERE user_id IS NULL;
        UPDATE delivery_jobs SET user_id = admin_id WHERE user_id IS NULL;
        
        -- Create a default API key for the admin user to preserve legacy scripts
        INSERT INTO api_keys (key, user_id) VALUES ('kestrel-dev-key', admin_id) ON CONFLICT DO NOTHING;
    END IF;
END $$;

-- 4. Enforce NOT NULL
ALTER TABLE subscriptions ALTER COLUMN user_id SET NOT NULL;
ALTER TABLE events ALTER COLUMN user_id SET NOT NULL;
ALTER TABLE delivery_jobs ALTER COLUMN user_id SET NOT NULL;

-- 5. Recreate system_stats to be multi-tenant
DROP TABLE IF EXISTS system_stats;
CREATE TABLE system_stats (
    user_id UUID REFERENCES users(id) ON DELETE CASCADE,
    key TEXT,
    value BIGINT NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, key)
);

-- Re-hydrate system_stats for all users
INSERT INTO system_stats (user_id, key, value)
SELECT user_id, 'total_events', COUNT(*) FROM events GROUP BY user_id;

INSERT INTO system_stats (user_id, key, value)
SELECT user_id, 'delivery_' || status, COUNT(*) FROM delivery_jobs GROUP BY user_id, status;

-- 6. Recreate triggers to pass user_id
CREATE OR REPLACE FUNCTION trg_events_stats() RETURNS TRIGGER AS $$
BEGIN
    IF TG_OP = 'INSERT' THEN
        INSERT INTO system_stats (user_id, key, value) VALUES (NEW.user_id, 'total_events', 1)
        ON CONFLICT (user_id, key) DO UPDATE SET value = system_stats.value + 1;
    ELSIF TG_OP = 'DELETE' THEN
        UPDATE system_stats SET value = value - 1 WHERE user_id = OLD.user_id AND key = 'total_events';
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION trg_delivery_jobs_stats() RETURNS TRIGGER AS $$
BEGIN
    IF TG_OP = 'INSERT' THEN
        INSERT INTO system_stats (user_id, key, value) VALUES (NEW.user_id, 'delivery_' || NEW.status, 1)
        ON CONFLICT (user_id, key) DO UPDATE SET value = system_stats.value + 1;
    ELSIF TG_OP = 'UPDATE' THEN
        IF OLD.status != NEW.status THEN
            UPDATE system_stats SET value = value - 1 WHERE user_id = OLD.user_id AND key = 'delivery_' || OLD.status;
            INSERT INTO system_stats (user_id, key, value) VALUES (NEW.user_id, 'delivery_' || NEW.status, 1)
            ON CONFLICT (user_id, key) DO UPDATE SET value = system_stats.value + 1;
        END IF;
    ELSIF TG_OP = 'DELETE' THEN
        UPDATE system_stats SET value = value - 1 WHERE user_id = OLD.user_id AND key = 'delivery_' || OLD.status;
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;
