-- What a mailbox owner decided about a sender, overruling the classifier.
--
-- The verdict engine judges every new sender and is sometimes wrong: it calls a
-- customer's private address personal, or an old friend business. Without a
-- place for a person's answer, the only way to correct it would be to re-run the
-- machine and hope — and the machine would overwrite the correction on the next
-- message from that sender anyway.
--
-- Per SEAT, not per workspace. A sender is personal to the person who knows
-- them: one rep's family member is another rep's customer, and a shared list
-- would let either overrule the other about their own correspondence.
SET LOCAL lock_timeout = '3s';

CREATE TABLE capture_sender_override (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    user_id uuid NOT NULL REFERENCES app_user(id) ON DELETE CASCADE,
    -- Folded to lower case by the writer, matching how every other capture
    -- surface stores an address.
    address text NOT NULL,
    decision text NOT NULL,
    -- What the machine had said, kept so the page can show "you overruled
    -- this" rather than only the current answer. Nullable: a person may decide
    -- about a sender the engine never reached.
    overruled_kind text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT capture_sender_override_decision_check
        CHECK (decision IN ('business', 'keep_out')),
    CONSTRAINT capture_sender_override_user_address_key UNIQUE (user_id, address)
);

-- The page's own read: one seat's decisions, newest first.
CREATE INDEX capture_sender_override_user_idx
    ON capture_sender_override (user_id, created_at DESC);
