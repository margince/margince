SET LOCAL lock_timeout = '5s';
ALTER TABLE custom_field VALIDATE CONSTRAINT custom_field_reportable_type_check;
