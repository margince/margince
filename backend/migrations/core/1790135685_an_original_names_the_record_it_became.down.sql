DROP INDEX IF EXISTS activity_raw_capture_id_idx;

ALTER TABLE activity DROP COLUMN IF EXISTS raw_capture_id;
