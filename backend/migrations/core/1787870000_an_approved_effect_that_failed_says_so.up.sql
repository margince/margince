-- An approved decision whose effect failed says so on the row.
--
-- The effect a decision releases runs AFTER the decision transaction commits,
-- because it writes through other modules' stores. When it fails the approval is
-- still approved: nothing un-decides it, the same row refuses a second decision
-- as already decided, and the work the human released never happened.
--
-- Until now that row was indistinguishable from a decision whose effect ran. It
-- is not pending, so the decision lane does not carry it; it names a human
-- decider, so the receipts lane does not either. A person pressed Accept, the
-- product told them it was accepted, and the email was never sent — with nothing
-- anywhere asking them to look again. The only trace was an error returned to
-- the request that decided it, seen once, by whoever happened to be looking.
--
-- effect_failed_at makes the row findable so a surface can carry it back to the
-- person who approved it. effect_failure is the sentence a reader is shown:
-- written from the error the executor returned, and by the same rule every other
-- message this product shows a client — what went wrong and what to do, never a
-- stack, a SQL statement or a table name.
SET LOCAL lock_timeout = '3s';

ALTER TABLE approval
    ADD COLUMN IF NOT EXISTS effect_failed_at timestamptz,
    ADD COLUMN IF NOT EXISTS effect_failure text;

-- A failure has a time and a reason, or it is not a failure. The pair moves
-- together or a reader gets a card with nothing on it.
ALTER TABLE approval
    ADD CONSTRAINT approval_effect_failure_is_stated
    CHECK ((effect_failed_at IS NULL) = (effect_failure IS NULL)) NOT VALID;

-- The lane reads the unredeemed rows newest-first and nothing else touches this
-- column, so the index carries only the rows that failed.
CREATE INDEX IF NOT EXISTS approval_effect_failed_idx
    ON approval (effect_failed_at DESC)
    WHERE effect_failed_at IS NOT NULL;
