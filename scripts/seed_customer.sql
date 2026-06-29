DO $$
DECLARE
    u_id uuid;
    s_id uuid;
BEGIN
    SELECT id INTO u_id FROM users WHERE email = 'dxdgremory11@customer.com';
    
    INSERT INTO subscriptions (user_id, endpoint_url, secret, event_types, is_active) 
    VALUES (u_id, 'https://api.dxdgremory.local/webhook', 'secret', '{order.created, payment.succeeded}', true)
    RETURNING id INTO s_id;

    -- Insert 500 random events
    WITH new_events AS (
        INSERT INTO events (type, payload, user_id)
        SELECT 'order.created', '{"order_id": "1234"}'::jsonb, u_id 
        FROM generate_series(1, 500)
        RETURNING id
    )
    -- Insert jobs for them
    INSERT INTO delivery_jobs (event_id, subscription_id, user_id, status)
    SELECT id, s_id, u_id, 'delivered' FROM new_events;
    
    -- Update stats
    INSERT INTO system_stats (key, value, user_id) VALUES ('total_events', 500, u_id)
    ON CONFLICT (key, user_id) DO UPDATE SET value = system_stats.value + 500;
    
    INSERT INTO system_stats (key, value, user_id) VALUES ('delivery_delivered', 450, u_id)
    ON CONFLICT (key, user_id) DO UPDATE SET value = system_stats.value + 450;
    
    INSERT INTO system_stats (key, value, user_id) VALUES ('delivery_pending', 50, u_id)
    ON CONFLICT (key, user_id) DO UPDATE SET value = system_stats.value + 50;
END $$;
