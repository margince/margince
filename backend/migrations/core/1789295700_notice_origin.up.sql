SET LOCAL lock_timeout = '3s';

ALTER TABLE notice ADD COLUMN origin jsonb;
