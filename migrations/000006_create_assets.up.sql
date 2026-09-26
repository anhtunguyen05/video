CREATE TABLE IF NOT EXISTS assets (
    id uuid PRIMARY KEY,
    video_id uuid NOT NULL REFERENCES videos(id) ON DELETE CASCADE,
    asset_type varchar(32) NOT NULL,
    variant varchar(32),
    object_key text NOT NULL,
    content_type varchar(128),
    size_bytes bigint,
    checksum varchar(128),
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT assets_type_check
        CHECK (asset_type IN ('SOURCE', 'THUMBNAIL', 'RENDITION', 'HLS_MASTER', 'HLS_VARIANT')),
    CONSTRAINT assets_size_check
        CHECK (size_bytes IS NULL OR size_bytes >= 0),
    CONSTRAINT assets_video_type_variant_unique
        UNIQUE (video_id, asset_type, variant)
);

CREATE INDEX IF NOT EXISTS assets_video_created_idx
    ON assets (video_id, created_at DESC);

INSERT INTO schema_migrations (version)
VALUES (6)
ON CONFLICT (version) DO NOTHING;
