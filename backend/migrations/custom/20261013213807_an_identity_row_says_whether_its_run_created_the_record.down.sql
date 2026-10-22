SET LOCAL lock_timeout = '3s';

-- A bound row maps a record no run created, so it has no place in the map
-- this migration found.
DELETE FROM import_record_map WHERE origin = 'bound';

ALTER TABLE import_record_map
    DROP CONSTRAINT import_record_map_origin_check,
    DROP COLUMN origin;
