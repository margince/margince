-- Put the check back as this migration found it: present and unvalidated.
SET LOCAL lock_timeout = '3s';

ALTER TABLE import_record_map
    DROP CONSTRAINT import_record_map_origin_check,
    ADD CONSTRAINT import_record_map_origin_check CHECK (origin IN ('created', 'bound')) NOT VALID;
