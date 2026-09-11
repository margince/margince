-- Dropping the marker loses only the sweep's bookkeeping. The payloads it
-- already slimmed stay slimmed and their stanzas still say so, so a re-applied
-- up migration re-reads every row once, which is correct and merely slow: a
-- part already replaced by its stanza has no encoded body left to locate, so
-- the sweep strips nothing and stamps the row.

SET LOCAL lock_timeout = '5s';

DROP INDEX IF EXISTS raw_capture_unslimmed_idx;
ALTER TABLE raw_capture DROP COLUMN IF EXISTS parts_slimmed_at;
