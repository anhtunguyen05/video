ALTER TABLE videos
    DROP CONSTRAINT IF EXISTS videos_status_check;

ALTER TABLE videos
    ADD CONSTRAINT videos_status_check
    CHECK (status IN ('CREATED', 'UPLOADING', 'UPLOADED', 'DELETED'));

CREATE TABLE IF NOT EXISTS video_uploads (
    id uuid PRIMARY KEY,
    video_id uuid NOT NULL REFERENCES videos(id),
    object_key text NOT NULL,
    status varchar(32) NOT NULL DEFAULT 'PENDING',
    expected_content_type varchar(128) NOT NULL,
    expected_size_bytes bigint NOT NULL,
    completed_size_bytes bigint,
    expires_at timestamptz NOT NULL,
    completed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT video_uploads_status_check
        CHECK (status IN ('PENDING', 'UPLOADING', 'COMPLETED', 'ABORTED', 'EXPIRED')),
    CONSTRAINT video_uploads_expected_size_check
        CHECK (expected_size_bytes > 0),
    CONSTRAINT video_uploads_completed_size_check
        CHECK (completed_size_bytes IS NULL OR completed_size_bytes >= 0)
);

CREATE INDEX IF NOT EXISTS video_uploads_video_created_idx
    ON video_uploads (video_id, created_at DESC);

CREATE INDEX IF NOT EXISTS video_uploads_expiry_idx
    ON video_uploads (status, expires_at);

CREATE UNIQUE INDEX IF NOT EXISTS video_uploads_active_video_idx
    ON video_uploads (video_id)
    WHERE status IN ('PENDING', 'UPLOADING');

INSERT INTO schema_migrations (version)
VALUES (3)
ON CONFLICT (version) DO NOTHING;
