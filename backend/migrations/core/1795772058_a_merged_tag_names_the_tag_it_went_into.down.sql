SET LOCAL lock_timeout = '3s';

ALTER TABLE tag DROP COLUMN IF EXISTS merged_into_id;
