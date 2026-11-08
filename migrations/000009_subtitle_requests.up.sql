ALTER TABLE video_uploads
    ADD COLUMN generate_subtitles boolean NOT NULL DEFAULT false;

-- Media API-owned projection. The subtitle service will own execution state.
CREATE TABLE subtitle_requests (
    id uuid PRIMARY KEY,
    video_id uuid NOT NULL REFERENCES videos(id),
    owner_id uuid NOT NULL,
    source_version integer NOT NULL CHECK (source_version > 0),
    language text NOT NULL CHECK (language = 'vi'),
    status text NOT NULL CHECK (status IN ('PENDING', 'PROCESSING', 'READY', 'NO_AUDIO', 'NO_SPEECH', 'FAILED')),
    output_object_key text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (video_id, source_version, language),
    CHECK ((status = 'READY') = (output_object_key IS NOT NULL))
);

INSERT INTO schema_migrations (version) VALUES (9)
ON CONFLICT (version) DO NOTHING;
