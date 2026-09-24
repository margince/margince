-- Put the constraint back the way this migration found it: present, unvalidated.
--
-- Postgres has no ALTER that un-validates, so the constraint is dropped and
-- re-added NOT VALID, which takes ACCESS EXCLUSIVE without a scan. The rows
-- still satisfy it; what is restored is the catalog's record of whether anybody
-- has checked.
SET LOCAL lock_timeout = '3s';

ALTER TABLE ai_call
  DROP CONSTRAINT ai_call_schema_downgrade_check,
  ADD CONSTRAINT ai_call_schema_downgrade_check
    CHECK (schema_downgrade = ANY (ARRAY[''::text, 'relaxed'::text, 'unenforced'::text, 'dropped'::text])) NOT VALID;
