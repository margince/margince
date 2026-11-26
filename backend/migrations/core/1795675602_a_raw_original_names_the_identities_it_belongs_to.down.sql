SET LOCAL lock_timeout = '3s';

ALTER TABLE raw_capture DROP COLUMN IF EXISTS channel_subjects;
