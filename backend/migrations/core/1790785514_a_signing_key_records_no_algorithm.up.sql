-- signing_key.alg could only ever be 'EdDSA', and the insert never named it, so
-- every row took the default. Nothing selects it.
--
-- A key's algorithm is worth recording for a rotation — and a CHECK admitting
-- exactly one value forbids the rotation it would be recorded for, so the column
-- documents an intent it also prevents. It comes back with the second algorithm,
-- written at insert and checked against the set the verifier accepts, which is
-- the version of it that would mean something.
SET LOCAL lock_timeout = '3s';

ALTER TABLE signing_key DROP CONSTRAINT workspace_signing_key_alg_check;
ALTER TABLE signing_key DROP COLUMN alg;
