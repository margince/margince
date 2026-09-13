SET LOCAL lock_timeout = '3s';

ALTER TABLE deal_correction ADD COLUMN accepted_at timestamptz;
ALTER TABLE stage_progression_outcome ADD COLUMN accepted_at timestamptz;
