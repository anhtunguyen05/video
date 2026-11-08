DROP TABLE IF EXISTS subtitle_requests;
ALTER TABLE video_uploads DROP COLUMN IF EXISTS generate_subtitles;
DELETE FROM schema_migrations WHERE version = 9;
