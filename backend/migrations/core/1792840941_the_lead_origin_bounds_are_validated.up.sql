-- The lead origin foreign key and detail bound added beside this file are
-- validated here, in a file of their own, so the scan runs under SHARE UPDATE
-- EXCLUSIVE and not under the ACCESS EXCLUSIVE the ALTER took.
SET LOCAL lock_timeout = '3s';

ALTER TABLE lead VALIDATE CONSTRAINT lead_origin_detail_bounded;
ALTER TABLE lead VALIDATE CONSTRAINT lead_origin_fkey;
