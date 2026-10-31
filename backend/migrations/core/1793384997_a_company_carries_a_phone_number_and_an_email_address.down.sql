-- Dropping the columns discards every company number and address. That is the
-- honest reverse: nothing else in the schema holds them.
SET LOCAL lock_timeout = '3s';
ALTER TABLE company DROP COLUMN phone, DROP COLUMN email;
