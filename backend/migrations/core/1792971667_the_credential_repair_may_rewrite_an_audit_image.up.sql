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
--
-- A changed image must carry the mask. The repair replaces a credential with
-- one marker, so the image it writes always holds that marker, and an image
-- rewritten to anything else is refused here.
--
-- A bound on LENGTH would be the wrong test. The marker is longer than a short
-- secret, so masking `Passwort: abc` lengthens the image, and a length test
-- would refuse the rewrite and leave that credential in the trail.
--
-- An image is also never conjured where the row held none, because a NULL image
-- quotes nothing and so has no credential to take out.
CREATE OR REPLACE FUNCTION audit_log_immutable() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
  IF TG_OP = 'UPDATE'
     AND current_setting('margince.audit_credential_mask', true) = 'on'
     AND (to_jsonb(NEW) - 'before' - 'after') = (to_jsonb(OLD) - 'before' - 'after')
     AND (NEW.before IS NOT DISTINCT FROM OLD.before
          OR (OLD.before IS NOT NULL AND NEW.before::text LIKE '%[masked]%'))
     AND (NEW.after IS NOT DISTINCT FROM OLD.after
          OR (OLD.after IS NOT NULL AND NEW.after::text LIKE '%[masked]%'))
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
