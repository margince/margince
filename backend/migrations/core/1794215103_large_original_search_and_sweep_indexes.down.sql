-- pgmigrate:no-transaction
DROP INDEX CONCURRENTLY IF EXISTS raw_capture_payload_offload_unchecked;
DROP INDEX CONCURRENTLY IF EXISTS raw_capture_payload_object_key;
DROP INDEX CONCURRENTLY IF EXISTS raw_capture_original_subject_hints;
DROP INDEX CONCURRENTLY IF EXISTS raw_capture_original_legacy_hints;
