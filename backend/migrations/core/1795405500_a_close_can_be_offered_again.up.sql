SET LOCAL lock_timeout = '3s';
ALTER TABLE deal_correction ADD COLUMN IF NOT EXISTS reoffered_at timestamptz;
