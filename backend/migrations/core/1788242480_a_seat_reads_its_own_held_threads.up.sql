-- The held-threads list asks one question: which threads is THIS seat holding.
--
-- The table's existing indexes answer neither half of it. capture_thread_verdict
-- is unique on (thread_key, user_id), which leads with the thread and so cannot
-- serve a scan by seat; and the due-work index leads with next_attempt_at and is
-- partial on pending rows, which is the opposite selection. Without this the
-- read scans the workspace-wide ledger and sorts, on a table that grows one row
-- per thread per seat for the life of the thread.
--
-- Partial on exactly what the list selects. A cleared thread is not held and is
-- never on this page, and the ledger keeps those rows forever — they are the
-- majority in a healthy installation, and an unfiltered index would carry all
-- of them to answer a question that excludes them.
-- The build blocks writes to capture_thread_verdict, which the capture sink
-- writes on every message it holds. Bounded so a migration that arrives while a
-- sync is running fails and is retried rather than queueing behind it and
-- holding every later capture out — an unbounded wait here is a deploy that
-- stops mail from being captured for as long as it takes.
SET LOCAL lock_timeout = '3s';

CREATE INDEX IF NOT EXISTS capture_thread_verdict_held_by_seat_idx
    ON capture_thread_verdict (user_id, status)
 WHERE status NOT IN ('cleared', 'shared_by_owner');
