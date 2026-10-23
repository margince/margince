SET LOCAL lock_timeout = '3s';

ALTER TABLE raw_capture DROP CONSTRAINT IF EXISTS raw_capture_parts_slim_outcome_known;
ALTER TABLE raw_capture DROP COLUMN IF EXISTS parts_slim_outcome;
