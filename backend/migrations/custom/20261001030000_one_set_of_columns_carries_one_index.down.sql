-- Put the constraint back exactly as the catalog held it.
SET LOCAL lock_timeout = '3s';

ALTER TABLE import_run ADD CONSTRAINT uq_import_run UNIQUE (id);
