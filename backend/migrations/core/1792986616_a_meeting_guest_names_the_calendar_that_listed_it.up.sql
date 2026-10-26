-- The seat whose calendar roster wrote this participant row. NULL is a row no
-- roster wrote: a member, an import or mail named it, and a roster sync never retires it.
SET LOCAL lock_timeout = '3s';
ALTER TABLE activity_participant ADD COLUMN roster_seat uuid
  REFERENCES app_user(id) ON DELETE SET NULL;
