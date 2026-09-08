-- A task minted from a meeting names that meeting.
--
-- The transcript flow writes its provenance as prose — "Lena Fischer committed
-- to this in the meeting transcript (line 6)" — and drops the id it had in
-- hand. A rep reading the task is told where the promise came from and given no
-- way to get there: the only route back is the record's history tab and an
-- exact-subject search.
--
-- Nullable, because almost no task has one: a task somebody typed came from
-- nowhere but them. ON DELETE SET NULL rather than CASCADE — a task outlives
-- the meeting it came from, and erasing the meeting must not take the work with
-- it.
SET LOCAL lock_timeout = '3s';
ALTER TABLE activity
    ADD COLUMN IF NOT EXISTS source_activity_id uuid
        REFERENCES activity (id) ON DELETE SET NULL;

-- Reading DOWN the edge — "which tasks came from this meeting" — is the query
-- the meeting's own page will want. Partial, because the column is null on
-- nearly every row.
CREATE INDEX IF NOT EXISTS idx_activity_source_activity
    ON activity (source_activity_id)
    WHERE source_activity_id IS NOT NULL;
