DO $$
DECLARE
    sub_id UUID;
    cust_id UUID;
BEGIN
    -- Get the customer user id
    SELECT id INTO cust_id FROM users WHERE email != 'admin@kestrel.local' LIMIT 1;
    IF cust_id IS NULL THEN
        RAISE EXCEPTION 'No customer user found!';
    END IF;

    -- Ensure we have a subscription for this customer
    SELECT id INTO sub_id FROM subscriptions WHERE user_id = cust_id LIMIT 1;
    IF sub_id IS NULL THEN
        INSERT INTO subscriptions (endpoint_url, secret, event_types, is_active, user_id)
        VALUES ('http://webhook-target:9999/webhook', 'secret', '{stress.test}', true, cust_id)
        RETURNING id INTO sub_id;
    END IF;

    RAISE NOTICE 'Inserting 1,000,000 events for customer %...', cust_id;

    -- Disable triggers to speed up bulk insert
    ALTER TABLE events DISABLE TRIGGER ALL;
    ALTER TABLE delivery_jobs DISABLE TRIGGER ALL;

    -- Insert in a loop to avoid blowing up memory with a single giant CTE
    FOR i IN 1..10 LOOP
        WITH new_events AS (
            INSERT INTO events (type, payload, user_id)
            SELECT 'stress.test', '{"amount": 100, "currency": "USD"}'::jsonb, cust_id
            FROM generate_series(1, 100000)
            RETURNING id
        )
        INSERT INTO delivery_jobs (event_id, subscription_id, status, user_id)
        SELECT id, sub_id, 'pending', cust_id
        FROM new_events;
        
        RAISE NOTICE 'Inserted batch % / 10', i;
    END LOOP;

    -- Re-enable triggers
    ALTER TABLE events ENABLE TRIGGER ALL;
    ALTER TABLE delivery_jobs ENABLE TRIGGER ALL;

    -- Manually update system_stats
    RAISE NOTICE 'Syncing system stats...';
    
    INSERT INTO system_stats (user_id, key, value)
    VALUES (cust_id, 'total_events', 1000000)
    ON CONFLICT (user_id, key) DO UPDATE SET value = system_stats.value + 1000000;

    INSERT INTO system_stats (user_id, key, value)
    VALUES (cust_id, 'delivery_pending', 1000000)
    ON CONFLICT (user_id, key) DO UPDATE SET value = system_stats.value + 1000000;

END $$;
