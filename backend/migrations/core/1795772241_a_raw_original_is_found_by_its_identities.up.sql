-- pgmigrate:no-transaction
-- Finds the raw originals of one identity without reading every row. Built while
-- the app still writes, because raw_capture is live.
DROP INDEX CONCURRENTLY IF EXISTS raw_capture_channel_subjects;
CREATE INDEX CONCURRENTLY raw_capture_channel_subjects ON raw_capture USING gin (channel_subjects)
    WHERE channel_subjects IS NOT NULL;
