DO $$
DECLARE
    u_id uuid;
    s_id uuid;
BEGIN
    INSERT INTO users (email, password_hash) 
    VALUES ('local@test.com', 'hash') 
    RETURNING id INTO u_id;
    
    INSERT INTO subscriptions (user_id, endpoint_url, secret, event_types, is_active) 
    VALUES (u_id, 'http://webhook-target:9999/webhook', 'secret', '{order.created, payment.succeeded}', true) 
    RETURNING id INTO s_id;
END $$;
