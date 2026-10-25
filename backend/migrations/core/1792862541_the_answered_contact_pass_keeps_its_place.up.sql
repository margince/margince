-- Where the reconciling pass that publishes answered contacts stopped.
--
-- The pass selects contacts owed publication in id order, 200 at a time. With
-- no position it read the same first 200 every tick, so 200 contacts whose
-- publication kept failing hid every contact behind them. The pass now walks
-- forward from this id and wraps to the start at the end.
--
-- One row per workspace database. Bookkeeping about a pass, not audited.

SET LOCAL lock_timeout = '5s';

CREATE TABLE capture_answered_sweep_cursor (
    singleton boolean PRIMARY KEY DEFAULT true,
    -- The last contact the pass inspected. NULL starts at the first contact.
    after_contact_id uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT capture_answered_sweep_cursor_one_row CHECK (singleton)
);
