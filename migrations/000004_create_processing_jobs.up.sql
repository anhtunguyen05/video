CREATE TABLE IF NOT EXISTS processing_jobs (
    id uuid PRIMARY KEY,
    video_id uuid NOT NULL REFERENCES videos(id),
    owner_id uuid NOT NULL,
    source_object_key text NOT NULL,
    processing_version integer NOT NULL,
    operation_key text NOT NULL,
    status varchar(32) NOT NULL DEFAULT 'QUEUED',
    attempt integer NOT NULL DEFAULT 0,
    max_attempts integer NOT NULL DEFAULT 3,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT processing_jobs_status_check
        CHECK (status IN ('QUEUED', 'RUNNING', 'SUCCEEDED', 'FAILED')),
    CONSTRAINT processing_jobs_version_check
        CHECK (processing_version > 0),
    CONSTRAINT processing_jobs_attempt_check
        CHECK (attempt >= 0 AND max_attempts > 0),
    CONSTRAINT processing_jobs_operation_key_unique UNIQUE (operation_key),
    CONSTRAINT processing_jobs_video_version_unique UNIQUE (video_id, processing_version)
);

CREATE INDEX IF NOT EXISTS processing_jobs_status_created_idx
    ON processing_jobs (status, created_at);

CREATE INDEX IF NOT EXISTS processing_jobs_video_idx
    ON processing_jobs (video_id, created_at DESC);

INSERT INTO schema_migrations (version)
VALUES (4)
ON CONFLICT (version) DO NOTHING;
