DROP TABLE IF EXISTS subtitle_outbox;
DROP TABLE IF EXISTS subtitle_runs;
DELETE FROM schema_migrations WHERE version = 11;
