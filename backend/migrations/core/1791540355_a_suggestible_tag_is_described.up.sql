-- The description check added beside this file is validated here, under SHARE
-- UPDATE EXCLUSIVE, which readers and writers pass.
SET LOCAL lock_timeout = '3s';

ALTER TABLE tag VALIDATE CONSTRAINT tag_suggestible_is_described;
