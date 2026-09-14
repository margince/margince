SET LOCAL lock_timeout = '3s';

ALTER TABLE stage_progression_outcome DROP COLUMN accepted_at;
ALTER TABLE deal_correction DROP COLUMN accepted_at;
