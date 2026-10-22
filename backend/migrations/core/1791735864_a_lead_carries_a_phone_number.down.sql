-- Dropping the column discards every number a lead carried. That is the honest
-- reverse: nothing else in the schema holds a lead's phone.
SET LOCAL lock_timeout = '3s';
ALTER TABLE lead DROP COLUMN phone;
