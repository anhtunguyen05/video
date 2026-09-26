CREATE TABLE IF NOT EXISTS renditions (
    id uuid PRIMARY KEY,
    video_id uuid NOT NULL REFERENCES videos(id) ON DELETE CASCADE,
    name varchar(32) NOT NULL,
    width integer NOT NULL,
    height integer NOT NULL,
    codec varchar(64) NOT NULL,
    bitrate_kbps integer,
    status varchar(32) NOT NULL DEFAULT 'PLANNED',
    object_prefix text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT renditions_name_check
        CHECK (name IN ('360p', '720p', '1080p')),
    CONSTRAINT renditions_dimensions_check
        CHECK (width > 0 AND height > 0 AND width % 2 = 0 AND height % 2 = 0),
    CONSTRAINT renditions_bitrate_check
        CHECK (bitrate_kbps IS NULL OR bitrate_kbps > 0),
    CONSTRAINT renditions_status_check
        CHECK (status IN ('PLANNED', 'PROCESSING', 'READY', 'FAILED')),
    CONSTRAINT renditions_video_name_unique UNIQUE (video_id, name)
);

CREATE INDEX IF NOT EXISTS renditions_video_created_idx
    ON renditions (video_id, created_at DESC);

ALTER TABLE processing_jobs
    DROP CONSTRAINT IF EXISTS processing_jobs_stage_check;

ALTER TABLE processing_jobs
    ADD CONSTRAINT processing_jobs_stage_check
    CHECK (stage IN (
        'VALIDATE_SOURCE', 'METADATA', 'THUMBNAIL', 'RENDITION_PLANNING',
        'TRANSCODING', 'PACKAGING'
    ));

INSERT INTO schema_migrations (version)
VALUES (7)
ON CONFLICT (version) DO NOTHING;
