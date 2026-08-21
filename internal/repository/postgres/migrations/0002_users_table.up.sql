CREATE TABLE IF NOT EXISTS users (
    id         TEXT PRIMARY KEY,
    city       TEXT NOT NULL,
    orders     INT NOT NULL DEFAULT 0,
    ltv_cents  BIGINT NOT NULL DEFAULT 0,
    signup_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    is_prime   BOOLEAN NOT NULL DEFAULT false
);

CREATE INDEX IF NOT EXISTS idx_users_city ON users (city);
CREATE INDEX IF NOT EXISTS idx_users_ltv  ON users (ltv_cents);
