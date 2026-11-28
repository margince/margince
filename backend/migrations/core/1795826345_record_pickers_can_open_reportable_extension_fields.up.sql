SET LOCAL lock_timeout = '3s';

ALTER TABLE custom_field DROP CONSTRAINT custom_field_reportable_type_check;
ALTER TABLE custom_field ADD CONSTRAINT custom_field_reportable_type_check
 CHECK (NOT reportable OR
        (object IN ('deal', 'company') AND type IN ('picklist', 'boolean')) OR
        (object = 'deal' AND type = 'record')) NOT VALID;
