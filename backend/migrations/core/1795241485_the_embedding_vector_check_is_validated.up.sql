-- The check added beside this file is validated here, under SHARE UPDATE
-- EXCLUSIVE, which readers and writers pass.
SET LOCAL lock_timeout = '3s';

ALTER TABLE embedding VALIDATE CONSTRAINT embedding_holds_one_vector;
