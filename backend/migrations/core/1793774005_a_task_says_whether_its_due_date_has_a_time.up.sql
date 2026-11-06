-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Whether a task's due_at names an agreed time of day, such as a callback hour,
-- or only the day it is due by. Every task written before this column existed
-- was due by a day, so it reads false and keeps showing its date alone.
--
-- A task with no due_at has no time either. The trigger keeps that true for
-- every writer, including the ones that clear due_at with their own SQL.
SET LOCAL lock_timeout = '3s';

ALTER TABLE activity ADD COLUMN due_has_time boolean NOT NULL DEFAULT false;

CREATE FUNCTION activity_due_time_follows_due_at() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF NEW.due_at IS NULL THEN
    NEW.due_has_time := false;
  END IF;
  RETURN NEW;
END;
$$;
CREATE TRIGGER activity_due_time_follows_due_at BEFORE INSERT OR UPDATE OF due_at, due_has_time ON activity
  FOR EACH ROW EXECUTE FUNCTION activity_due_time_follows_due_at();
