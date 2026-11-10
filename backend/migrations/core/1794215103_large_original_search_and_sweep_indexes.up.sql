-- pgmigrate:no-transaction
DROP INDEX CONCURRENTLY IF EXISTS raw_capture_payload_object_key;
CREATE INDEX CONCURRENTLY raw_capture_payload_object_key ON raw_capture ((payload_object->>'key'))
 WHERE payload_object IS NOT NULL;
DROP INDEX CONCURRENTLY IF EXISTS raw_capture_payload_offload_unchecked;
CREATE INDEX CONCURRENTLY raw_capture_payload_offload_unchecked ON raw_capture(id)
 WHERE payload_offload_checked_at IS NULL AND payload_object IS NULL;
DROP INDEX CONCURRENTLY IF EXISTS raw_capture_original_subject_hints;
CREATE INDEX CONCURRENTLY raw_capture_original_subject_hints ON raw_capture USING gin(payload_object jsonb_path_ops)
 WHERE payload_object IS NOT NULL;
DROP INDEX CONCURRENTLY IF EXISTS raw_capture_original_legacy_hints;
CREATE INDEX CONCURRENTLY raw_capture_original_legacy_hints ON raw_capture(id)
 WHERE payload_object IS NOT NULL AND (
  (payload_object->'subjects'->>'version') IS DISTINCT FROM '1'
  OR jsonb_typeof(payload_object->'subjects'->'email_bits') IS DISTINCT FROM 'array'
  OR coalesce(payload_object->'subjects'->>'email_saturated','false') <> 'false');
