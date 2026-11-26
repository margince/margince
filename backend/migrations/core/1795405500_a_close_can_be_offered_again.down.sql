SET LOCAL lock_timeout = '3s';
ALTER TABLE deal_correction DROP COLUMN IF EXISTS reoffered_at;
