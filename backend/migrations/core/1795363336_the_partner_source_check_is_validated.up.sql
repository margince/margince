-- The partner check added before this file is validated here, under SHARE
-- UPDATE EXCLUSIVE, which readers and writers pass.
SET LOCAL lock_timeout = '3s';

ALTER TABLE partner VALIDATE CONSTRAINT partner_source_check;
