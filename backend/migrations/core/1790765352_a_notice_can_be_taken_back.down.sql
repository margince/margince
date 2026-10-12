SET LOCAL lock_timeout = '3s';

ALTER TABLE notice DROP COLUMN retracted_at;
