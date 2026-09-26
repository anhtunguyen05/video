ALTER TABLE videos
    DROP CONSTRAINT IF EXISTS videos_status_check;

ALTER TABLE videos
    ADD CONSTRAINT videos_status_check
    CHECK (status IN (
        'CREATED', 'UPLOADING', 'UPLOADED', 'QUEUED',
        'PROCESSING', 'READY', 'FAILED', 'DELETED'
    ));

ALTER TABLE processing_jobs
    ADD COLUMN IF NOT EXISTS stage varchar(32) NOT NULL DEFAULT 'METADATA',
    ADD COLUMN IF NOT EXISTS started_at timestamptz,
    ADD COLUMN IF NOT EXISTS finished_at timestamptz,
    ADD COLUMN IF NOT EXISTS next_retry_at timestamptz,
    ADD COLUMN IF NOT EXISTS error_code varchar(64),
    ADD COLUMN IF NOT EXISTS error_message text;

ALTER TABLE processing_jobs
    DROP CONSTRAINT IF EXISTS processing_jobs_stage_check;

ALTER TABLE processing_jobs
    ADD CONSTRAINT processing_jobs_stage_check
    CHECK (stage IN ('VALIDATE_SOURCE', 'METADATA', 'THUMBNAIL', 'TRANSCODING', 'PACKAGING'));

INSERT INTO schema_migrations (version)
VALUES (5)
ON CONFLICT (version) DO NOTHING;
