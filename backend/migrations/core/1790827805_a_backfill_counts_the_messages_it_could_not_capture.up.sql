-- A mailbox import no longer ends on one message it cannot capture. The page
-- skips that message and carries on, and the run counts it here, so the number
-- of messages that failed stays visible beside the ones skipped on purpose.
--
-- window_restarts counts how often the run walked its window again from the top
-- because the provider rejected the page token it stopped at. One restart is
-- allowed per run (a resume starts the count again); a provider that rejects
-- every token would otherwise keep the run walking forever.
SET LOCAL lock_timeout = '5s';

ALTER TABLE capture_backfill
    ADD COLUMN failed integer DEFAULT 0 NOT NULL,
    ADD COLUMN window_restarts integer DEFAULT 0 NOT NULL;
