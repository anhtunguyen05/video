DROP TABLE IF EXISTS video_uploads;

ALTER TABLE videos
    DROP CONSTRAINT IF EXISTS videos_status_check;

ALTER TABLE videos
    ADD CONSTRAINT videos_status_check
    CHECK (status IN ('CREATED', 'DELETED'));

DELETE FROM schema_migrations
WHERE version = 3;
