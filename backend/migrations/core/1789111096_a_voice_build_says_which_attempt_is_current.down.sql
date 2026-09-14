-- Same bound as the up: dropping a column takes ACCESS EXCLUSIVE too, and an
-- unbounded wait behind one open reader stalls every write to the table.
SET LOCAL lock_timeout = '3s';

ALTER TABLE voice_build
  DROP COLUMN attempt,
  DROP COLUMN attempt_at;
