SET LOCAL lock_timeout = '3s';

-- A bound row maps a ref to a record the file named by id and only corrected,
-- so undo must not archive it. Every row written before this was a creation.
ALTER TABLE import_record_map
    ADD COLUMN origin text NOT NULL DEFAULT 'created';

-- NOT VALID so the add does not scan under ACCESS EXCLUSIVE; the next file validates it.
ALTER TABLE import_record_map
    ADD CONSTRAINT import_record_map_origin_check CHECK (origin IN ('created', 'bound')) NOT VALID;
