-- Seeds ~10,000 mock users across a handful of cities for POC demos.
-- Deterministic: given the same schema, running this twice is a no-op thanks
-- to the ON CONFLICT clause.
INSERT INTO users (id, city, orders, ltv_cents, signup_at, is_prime)
SELECT
    'u_' || lpad(gs::text, 6, '0') AS id,
    (ARRAY['Delhi','Bangalore','Mumbai','Hyderabad','Chennai','Kolkata'])[1 + (gs % 6)] AS city,
    (gs % 50) AS orders,
    ((gs * 137) % 2000000)::bigint AS ltv_cents,
    now() - ((gs % 365) || ' days')::interval AS signup_at,
    (gs % 7 = 0) AS is_prime
FROM generate_series(1, 10000) AS gs
ON CONFLICT (id) DO NOTHING;
