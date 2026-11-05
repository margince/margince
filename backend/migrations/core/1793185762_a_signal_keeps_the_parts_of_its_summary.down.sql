SET LOCAL lock_timeout = '3s';

ALTER TABLE signal DROP COLUMN IF EXISTS summary_params;
