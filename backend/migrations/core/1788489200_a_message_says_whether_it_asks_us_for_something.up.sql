-- Whether an inbound message asks its recipient side for something.
--
-- The waiting queue can prove a customer wrote and nobody replied. It cannot
-- tell an unanswered question from a report, a receipt or a notification, and
-- the difference is the whole of what a rep wants to know. This column records a
-- model's judgement of that, per message.
--
-- WORKSPACE-GLOBAL, not per reader. The question is a property of the message —
-- does it ask the recipient side for something — and who owes the answer is
-- already resolved elsewhere, from the record the thread is filed under. A
-- per-reader verdict would be a second answer to a question the queue already
-- has a first answer for.
--
-- The verdict ROUTES ATTENTION and nothing more, exactly as capture_label does:
-- it may move a row's band within the queue and may never delete, archive or
-- suppress one. Unlike capture_label, setting it is audited and evented — it is
-- a model-derived claim about a customer's message, and "which records did the
-- classifier touch" has to stay answerable from audit_log.
--
-- Existing rows keep NULL, which reads as unjudged. Absence of a verdict is
-- never evidence: an unjudged message ranks exactly as it does today.
SET LOCAL lock_timeout = '3s';

ALTER TABLE activity
    ADD COLUMN IF NOT EXISTS owed_verdict text,
    ADD COLUMN IF NOT EXISTS owed_verdict_at timestamptz;

-- NOT VALID, then validated separately, and the distinction is the whole reason
-- this is two statements rather than one.
--
-- A plain ADD CONSTRAINT ... CHECK scans every existing row while holding ACCESS
-- EXCLUSIVE, which on a table the size of activity locks out every reader and
-- writer for the length of the scan. lock_timeout does not help: it bounds how
-- long we WAIT for the lock, never how long we hold it. NOT VALID takes the
-- strong lock only long enough to record the constraint, and VALIDATE CONSTRAINT
-- then does the scan under SHARE UPDATE EXCLUSIVE, which readers and writers
-- pass.
--
-- The rows being validated all carry NULL, since nothing has written the column
-- yet, so both constraints hold by construction — but the shape is what a later
-- migration on this table should copy, and getting it right when it is free is
-- cheaper than remembering when it is not.
ALTER TABLE activity
    ADD CONSTRAINT activity_owed_verdict_check
    CHECK (owed_verdict IS NULL OR owed_verdict IN ('asks_us', 'informs_us'))
    NOT VALID;

-- Both together or neither: a verdict with no moment cannot be aged out, and a
-- moment with no verdict claims a judgement that was never made.
ALTER TABLE activity
    ADD CONSTRAINT activity_owed_verdict_stamped
    CHECK ((owed_verdict IS NULL) = (owed_verdict_at IS NULL))
    NOT VALID;

ALTER TABLE activity VALIDATE CONSTRAINT activity_owed_verdict_check;
ALTER TABLE activity VALIDATE CONSTRAINT activity_owed_verdict_stamped;

COMMENT ON COLUMN activity.owed_verdict IS
    'Whether this inbound message asks its recipient side for something. Routes attention only; never hides a row. NULL means unjudged.';

-- The classifier's backlog: inbound correspondence nobody has judged yet.
--
-- Partial, so it indexes the work rather than the archive — the same shape
-- idx_activity_unlabeled uses for the capture-label backlog beside it. The
-- audience and hold clauses are here because a message the worklist may not open
-- is not one to spend a model call on.
CREATE INDEX IF NOT EXISTS idx_activity_unjudged
    ON activity (occurred_at)
    WHERE owed_verdict IS NULL
      AND direction = 'inbound'
      AND kind IN ('email', 'message')
      AND archived_at IS NULL
      AND audience = 'workspace'
      AND restricted_at IS NULL;
