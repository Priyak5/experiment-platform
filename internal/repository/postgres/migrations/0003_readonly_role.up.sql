DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'cohort_reader') THEN
        CREATE ROLE cohort_reader LOGIN PASSWORD 'cohort_reader';
    END IF;
END $$;

GRANT CONNECT ON DATABASE cohort TO cohort_reader;
GRANT USAGE ON SCHEMA public TO cohort_reader;
GRANT SELECT ON ALL TABLES IN SCHEMA public TO cohort_reader;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT ON TABLES TO cohort_reader;
ALTER ROLE cohort_reader SET default_transaction_read_only = on;
