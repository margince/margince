-- The two fork-owned tables, back to the word core used to use.

UPDATE import_record_map SET object = 'organization' WHERE object = 'company';
UPDATE mirror_visibility SET object_class = 'organization' WHERE object_class = 'company';
