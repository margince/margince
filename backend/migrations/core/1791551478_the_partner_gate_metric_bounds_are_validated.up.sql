-- The validation of the constraint added in the previous migration, which
-- repaired the rows that would have failed it.
SET LOCAL lock_timeout = '3s';

ALTER TABLE partner VALIDATE CONSTRAINT partner_certified_staff_check;
