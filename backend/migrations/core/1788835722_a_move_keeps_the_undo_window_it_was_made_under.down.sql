SET LOCAL lock_timeout = '3s';

ALTER TABLE stage_progression_outcome
  DROP COLUMN IF EXISTS undo_window_hours;
