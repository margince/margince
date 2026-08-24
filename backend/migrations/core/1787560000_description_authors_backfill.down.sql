-- Removing the backfilled stamps takes only the rows this migration could have
-- written: a description provenance row whose captured_at equals the
-- organization's own created_at, which is how the up half dated them. A stamp
-- written since by an edit carries the time of that edit and stays.
DELETE FROM field_provenance fp
USING organization o
WHERE fp.object_type = 'organization'
  AND fp.object_id = o.id
  AND fp.field_name = 'description'
  AND fp.captured_at = o.created_at;
