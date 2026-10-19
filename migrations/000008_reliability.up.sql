ALTER TABLE processing_jobs
    ADD COLUMN run_id uuid,
    ADD COLUMN lease_until timestamptz,
    ADD COLUMN original_message bytea;

ALTER TABLE processing_jobs
    DROP CONSTRAINT processing_jobs_attempt_check;
ALTER TABLE processing_jobs
    ADD CONSTRAINT processing_jobs_attempt_check
    CHECK (attempt >= 0 AND max_attempts > 0 AND attempt <= max_attempts);
ALTER TABLE processing_jobs ALTER COLUMN max_attempts SET DEFAULT 4;
UPDATE processing_jobs SET status = 'QUEUED', run_id = NULL, lease_until = NULL,
    next_retry_at = now(), max_attempts = GREATEST(max_attempts, 4)
WHERE status = 'RUNNING';
UPDATE processing_jobs SET max_attempts = 4
WHERE status = 'QUEUED' AND attempt <= 4;
CREATE INDEX processing_jobs_due_idx ON processing_jobs (next_retry_at, created_at)
    WHERE status = 'QUEUED';
CREATE INDEX processing_jobs_lease_idx ON processing_jobs (lease_until)
    WHERE status = 'RUNNING';

CREATE TABLE processing_operations (
    operation_key text PRIMARY KEY,
    job_id uuid NOT NULL REFERENCES processing_jobs(id) ON DELETE CASCADE,
    stage varchar(32) NOT NULL,
    variant varchar(32) NOT NULL DEFAULT '',
    status varchar(16) NOT NULL CHECK (status IN ('RUNNING', 'SUCCEEDED', 'FAILED')),
    run_id uuid NOT NULL,
    object_key text,
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE processing_runs (
    id uuid PRIMARY KEY,
    job_id uuid NOT NULL REFERENCES processing_jobs(id) ON DELETE CASCADE,
    object_prefix text NOT NULL UNIQUE,
    status varchar(16) NOT NULL CHECK (status IN ('RUNNING', 'FAILED', 'SUCCEEDED')),
    created_at timestamptz NOT NULL DEFAULT now(),
    finished_at timestamptz,
    cleaned_at timestamptz
);
CREATE INDEX processing_runs_cleanup_idx ON processing_runs (finished_at)
    WHERE status = 'FAILED' AND cleaned_at IS NULL;

CREATE TABLE outbox_events (
    id uuid PRIMARY KEY,
    event_key text NOT NULL UNIQUE,
    exchange_name text NOT NULL,
    routing_key text NOT NULL,
    payload jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    published_at timestamptz,
    attempt_count integer NOT NULL DEFAULT 0,
    next_attempt_at timestamptz NOT NULL DEFAULT now(),
    locked_until timestamptz
);
CREATE INDEX outbox_events_due_idx ON outbox_events (next_attempt_at)
    WHERE published_at IS NULL;

CREATE TABLE processing_dead_letters (
    id uuid PRIMARY KEY,
    dedupe_key text NOT NULL UNIQUE,
    job_id uuid REFERENCES processing_jobs(id) ON DELETE SET NULL,
    original_message bytea NOT NULL,
    error_code varchar(64) NOT NULL,
    error_message text NOT NULL,
    attempt integer NOT NULL,
    failed_at timestamptz NOT NULL,
    CONSTRAINT processing_dead_letters_job_unique UNIQUE (job_id)
);

INSERT INTO schema_migrations (version) VALUES (8)
ON CONFLICT (version) DO NOTHING;
