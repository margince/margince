-- A mailbox import no longer ends on one message it cannot capture. The page
-- skips that message and carries on, and the run counts it here, so the number
-- of messages that failed stays visible beside the ones skipped on purpose.
SET LOCAL lock_timeout = '5s';

ALTER TABLE capture_backfill
    ADD COLUMN failed integer DEFAULT 0 NOT NULL;
