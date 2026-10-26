-- audit_log stays append-only, with one named exception.
--
-- The repair of credentials stored before the masking rules has to reach the
-- `before` and `after` images of an edit to a mail body: a password written
-- into them in clear could never be taken out again, and the trail is the one
-- place no live writer can reach.
--
-- The exception is as narrow as it can be made. A DELETE is still refused
-- outright. An UPDATE is admitted only while the repair's own setting is on,
-- and only when every column except the two images is unchanged.
--
-- That last test is written as a comparison of the whole row rather than a list
-- of columns, so a column added later is protected without this function being
-- remembered.
CREATE OR REPLACE FUNCTION audit_log_immutable() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
  IF TG_OP = 'UPDATE'
     AND current_setting('margince.audit_credential_repair', true) = 'on'
     AND (to_jsonb(NEW) - 'before' - 'after') = (to_jsonb(OLD) - 'before' - 'after')
  THEN
    RETURN NEW;
  END IF;
  RAISE EXCEPTION 'audit_log is append-only (attempted % on row %)', TG_OP, OLD.id
    USING ERRCODE = 'check_violation';
END; $$;

-- The privilege is as narrow as the trigger's test, so neither stands alone.
-- A column-level grant cannot reach `action` or `entity_id` whatever the
-- setting says, and the trigger refuses the two it can reach unless the repair
-- is running.
GRANT UPDATE (before, after) ON TABLE audit_log TO margince_app;
