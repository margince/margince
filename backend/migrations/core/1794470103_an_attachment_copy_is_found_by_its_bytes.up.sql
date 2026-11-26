-- pgmigrate:no-transaction
-- A captured file whose bytes a row already holds points at that row's object
-- instead of storing a second copy (activities.storedCopies). Whether any row
-- still names an object is then asked by key, by erasure, by that capture and
-- by the stored-object reaper. Built concurrently, so attachment writes are
-- never blocked. Each drop clears an invalid index a failed build left behind.
DROP INDEX CONCURRENTLY IF EXISTS idx_attachment_held_copy;
CREATE INDEX CONCURRENTLY idx_attachment_held_copy ON attachment (checksum, byte_size)
    WHERE checksum IS NOT NULL AND NOT bytes_withheld;
DROP INDEX CONCURRENTLY IF EXISTS idx_attachment_storage_key;
CREATE INDEX CONCURRENTLY idx_attachment_storage_key ON attachment (storage_key)
    WHERE storage_key <> '';
