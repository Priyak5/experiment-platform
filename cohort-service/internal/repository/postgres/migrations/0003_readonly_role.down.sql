REVOKE ALL ON ALL TABLES IN SCHEMA public FROM cohort_reader;
REVOKE ALL ON SCHEMA public FROM cohort_reader;
REVOKE ALL ON DATABASE cohort FROM cohort_reader;
DROP ROLE IF EXISTS cohort_reader;
