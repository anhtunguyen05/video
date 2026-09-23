CREATE TABLE IF NOT EXISTS videos (
    id uuid PRIMARY KEY,
    owner_id uuid NOT NULL,
    title varchar(255) NOT NULL,
    original_filename varchar(255),
    status varchar(32) NOT NULL DEFAULT 'CREATED',
    source_object_key text,
    source_size_bytes bigint,
    source_container varchar(32),
    source_codec varchar(64),
    duration_ms bigint,
    width integer,
    height integer,
    frame_rate numeric(10,4),
    processing_version integer NOT NULL DEFAULT 1,
    failure_code varchar(64),
    failure_message text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz,
    CONSTRAINT videos_status_check CHECK (status IN ('CREATED', 'DELETED'))
);

CREATE INDEX IF NOT EXISTS videos_owner_created_idx
    ON videos (owner_id, created_at DESC);

CREATE INDEX IF NOT EXISTS videos_status_idx
    ON videos (status);

INSERT INTO schema_migrations (version)
VALUES (2)
ON CONFLICT (version) DO NOTHING;
