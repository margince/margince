-- The calendar-method check added beside this file is validated here, under SHARE
-- UPDATE EXCLUSIVE, which readers and writers pass.
SET LOCAL lock_timeout = '3s';

ALTER TABLE activity VALIDATE CONSTRAINT activity_calendar_method_check;
