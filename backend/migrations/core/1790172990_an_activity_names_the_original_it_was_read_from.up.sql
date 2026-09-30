SET LOCAL lock_timeout = '5s';

-- The link between a captured message and the provider original it was read
-- from, so a purge can follow it instead of guessing at it.
--
-- The retention sweep found the original by the natural-key join
-- (r.source_system = a.source_system AND r.source_id = a.source_id). That is
-- true for mail and false for every other lane, because raw_capture has two
-- writers and they key differently: the mail sink stores the domain natural
-- key, while InsertRawCaptureTx stores the PROVIDER'S REDELIVERY key — which
-- is the right value for the redelivery question it exists to answer, and is
-- not the activity's key. The join matched zero rows for a channel capture,
-- reported success, and the verbatim original survived every retention policy
-- and came back in an Art. 15 package.
--
-- The link was already established at ingest and thrown away: the Telegram
-- worker is handed the raw_capture id as a job argument and never stored it.
-- Persisting it makes the purge exact for every lane rather than for the one
-- whose keys happen to agree, which is what stops this recurring on the next
-- non-mail connector.
ALTER TABLE activity ADD COLUMN raw_capture_id uuid;

-- ON DELETE SET NULL, because the purge is the thing that deletes the parent.
-- CASCADE would destroy the redacted activity along with the original — the
-- timeline entry a reader is entitled to keep — and NO ACTION would make the
-- purge fail on exactly the rows it exists to clear.
ALTER TABLE activity
    ADD CONSTRAINT activity_raw_capture_id_fkey
    FOREIGN KEY (raw_capture_id) REFERENCES raw_capture (id) ON DELETE SET NULL;

-- The purge reads activity ids and needs their originals; nothing reads the
-- other direction, so the index is on the child side only. Partial, because
-- the column is NULL for every row written before this migration and for every
-- activity that never came from a provider original.
CREATE INDEX idx_activity_raw_capture ON activity (raw_capture_id)
    WHERE raw_capture_id IS NOT NULL;
