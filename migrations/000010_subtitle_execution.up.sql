-- Subtitle-service-owned execution state, independent of the media projection.
CREATE TABLE subtitle_jobs (
    id uuid PRIMARY KEY,
    video_id uuid NOT NULL,
    owner_id uuid NOT NULL,
    source_object_key text NOT NULL,
    source_version integer NOT NULL CHECK (source_version > 0),
    language text NOT NULL CHECK (language = 'vi'),
    correlation_id text NOT NULL,
    traceparent text,
    status text NOT NULL DEFAULT 'QUEUED' CHECK (status IN ('QUEUED', 'RUNNING', 'SUCCEEDED', 'FAILED')),
    attempt integer NOT NULL DEFAULT 0 CHECK (attempt >= 0),
    run_id uuid,
    lease_until timestamptz,
    next_retry_at timestamptz NOT NULL DEFAULT now(),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (video_id, source_version, language)
);
CREATE INDEX subtitle_jobs_due_idx ON subtitle_jobs (next_retry_at, created_at) WHERE status = 'QUEUED';
CREATE INDEX subtitle_jobs_lease_idx ON subtitle_jobs (lease_until) WHERE status = 'RUNNING';

INSERT INTO schema_migrations (version) VALUES (10)
ON CONFLICT (version) DO NOTHING;
