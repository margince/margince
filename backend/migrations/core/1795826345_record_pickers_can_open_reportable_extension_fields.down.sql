SET LOCAL lock_timeout = '3s';

DO $$ BEGIN
 IF EXISTS (SELECT 1 FROM custom_field WHERE reportable AND type = 'record') THEN
  RAISE EXCEPTION 'Turn off reporting for record-picker fields before downgrading';
 END IF;
END $$;
ALTER TABLE custom_field DROP CONSTRAINT custom_field_reportable_type_check;
ALTER TABLE custom_field ADD CONSTRAINT custom_field_reportable_type_check
 CHECK (NOT reportable OR (object IN ('deal', 'company') AND type IN ('picklist', 'boolean')));
