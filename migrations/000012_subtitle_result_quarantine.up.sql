CREATE TABLE subtitle_result_quarantine (
    digest text PRIMARY KEY,
    payload bytea NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
INSERT INTO schema_migrations (version) VALUES (12) ON CONFLICT (version) DO NOTHING;
