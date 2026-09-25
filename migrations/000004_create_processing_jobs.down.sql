DROP TABLE IF EXISTS processing_jobs;

DELETE FROM schema_migrations
WHERE version = 4;
