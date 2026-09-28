SET LOCAL lock_timeout = '5s';

ALTER TABLE bulk_operation DROP COLUMN IF EXISTS requested_for;
