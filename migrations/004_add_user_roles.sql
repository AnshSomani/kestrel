ALTER TABLE users ADD COLUMN IF NOT EXISTS role TEXT NOT NULL DEFAULT 'customer';
UPDATE users SET role = 'admin' WHERE email = 'admin@kestrel.local';
