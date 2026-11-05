SET LOCAL lock_timeout = '3s';

ALTER TABLE capture_import DROP COLUMN source_occurred_at;

DROP TABLE activity_commitment_admission;
