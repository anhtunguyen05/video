ALTER TABLE videos
    DROP CONSTRAINT IF EXISTS videos_status_check;

ALTER TABLE videos
    ADD CONSTRAINT videos_status_check
    CHECK (status IN ('CREATED', 'UPLOADING', 'UPLOADED', 'DELETED'));

ALTER TABLE processing_jobs
    DROP CONSTRAINT IF EXISTS processing_jobs_stage_check;

ALTER TABLE processing_jobs
    DROP COLUMN IF EXISTS stage,
    DROP COLUMN IF EXISTS started_at,
    DROP COLUMN IF EXISTS finished_at,
    DROP COLUMN IF EXISTS next_retry_at,
    DROP COLUMN IF EXISTS error_code,
    DROP COLUMN IF EXISTS error_message;

DELETE FROM schema_migrations WHERE version = 5;
