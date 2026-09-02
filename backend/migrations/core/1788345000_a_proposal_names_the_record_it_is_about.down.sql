SET LOCAL lock_timeout = '3s';

ALTER TABLE approval DROP COLUMN target_label;
