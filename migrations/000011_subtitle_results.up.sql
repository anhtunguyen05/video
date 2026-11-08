CREATE TABLE subtitle_runs (
    id uuid PRIMARY KEY,
    job_id uuid NOT NULL REFERENCES subtitle_jobs(id),
    object_key text NOT NULL UNIQUE,
    status text NOT NULL CHECK (status IN ('RUNNING', 'FAILED', 'SUCCEEDED')),
    updated_at timestamptz NOT NULL DEFAULT now(),
    cleaned_at timestamptz
);
CREATE INDEX subtitle_runs_cleanup_idx ON subtitle_runs (updated_at) WHERE status = 'FAILED' AND cleaned_at IS NULL;

CREATE TABLE subtitle_outbox (
    id uuid PRIMARY KEY,
    job_id uuid NOT NULL REFERENCES subtitle_jobs(id),
    payload jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    published_at timestamptz,
    locked_until timestamptz
);
CREATE INDEX subtitle_outbox_due_idx ON subtitle_outbox (created_at) WHERE published_at IS NULL;
INSERT INTO schema_migrations (version) VALUES (11) ON CONFLICT (version) DO NOTHING;
