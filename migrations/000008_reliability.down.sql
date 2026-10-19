DROP TABLE IF EXISTS processing_dead_letters;
DROP TABLE IF EXISTS outbox_events;
DROP TABLE IF EXISTS processing_runs;
DROP TABLE IF EXISTS processing_operations;
DROP INDEX IF EXISTS processing_jobs_lease_idx;
DROP INDEX IF EXISTS processing_jobs_due_idx;
ALTER TABLE processing_jobs DROP COLUMN IF EXISTS original_message;
ALTER TABLE processing_jobs DROP COLUMN IF EXISTS lease_until;
ALTER TABLE processing_jobs DROP COLUMN IF EXISTS run_id;
ALTER TABLE processing_jobs ALTER COLUMN max_attempts SET DEFAULT 3;
ALTER TABLE processing_jobs DROP CONSTRAINT processing_jobs_attempt_check;
ALTER TABLE processing_jobs ADD CONSTRAINT processing_jobs_attempt_check
    CHECK (attempt >= 0 AND max_attempts > 0);
DELETE FROM schema_migrations WHERE version = 8;
