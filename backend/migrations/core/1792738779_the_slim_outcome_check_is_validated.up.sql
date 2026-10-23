SET LOCAL lock_timeout = '3s';

ALTER TABLE raw_capture VALIDATE CONSTRAINT raw_capture_parts_slim_outcome_known;
