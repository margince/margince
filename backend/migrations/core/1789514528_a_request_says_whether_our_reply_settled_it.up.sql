-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '2s';

-- What our own reply did to a request somebody made of us.
--
-- The owed verdict on the activity says whether an inbound message ASKS for
-- something. It is written once, from that message alone, and nothing revisits
-- it — so a request answered an hour later still reads as owed forever, which
-- is the defect this table closes.
--
-- One row per REQUEST, not per reply: the question is "is this still owed",
-- and that has one answer at a time. judged_through_activity_id is the outbound
-- message the answer was reached over, and it is what makes the row safe to
-- replace — a later reply is new evidence about the same question, so the pass
-- re-asks and overwrites rather than accumulating opinions nobody reconciles.
-- The prior state is not lost: the write is audited with a before-image, which
-- is where "what did it say yesterday" is answered.
CREATE TABLE IF NOT EXISTS activity_request_settlement (
    request_activity_id uuid PRIMARY KEY REFERENCES activity(id) ON DELETE CASCADE,
    -- The outbound reply this judgement read through. A newer one re-arms the
    -- question; the same one never asks it twice.
    judged_through_activity_id uuid NOT NULL REFERENCES activity(id) ON DELETE CASCADE,
    verdict text NOT NULL,
    -- What is STILL owed, in the reader's own seat ("Send the quote"), when the
    -- reply left something undone. Model prose about a customer's conversation,
    -- which is why it carries an erasure obligation like every other stored
    -- reading of correspondence.
    remaining text,
    -- A deadline only when our own words named one. Never computed.
    due_at timestamptz,
    confidence numeric NOT NULL,
    decided_at timestamptz NOT NULL DEFAULT now(),
    -- The classifier version that reached it, as the owed and reply verdicts
    -- record theirs.
    decided_by text NOT NULL,
    CONSTRAINT activity_request_settlement_verdict
        CHECK (verdict IN ('settled', 'still_owed', 'unsure')),
    -- Prose belongs to still_owed and to nothing else. A settled request with
    -- something remaining is two answers to one question, and an unsure one
    -- has not answered it at all.
    CONSTRAINT activity_request_settlement_remaining
        CHECK (verdict = 'still_owed' OR (remaining IS NULL AND due_at IS NULL)),
    CONSTRAINT activity_request_settlement_confidence
        CHECK (confidence >= 0 AND confidence <= 1)
);

COMMENT ON TABLE activity_request_settlement IS
    'Whether our own reply settled the request an inbound message made, judged over the thread once we have written back.';

-- No index for the trigger set. idx_activity_thread_reply_seek already covers
-- it — (thread_key, kind, channel_provider, direction, occurred_at, id) over
-- live mail is precisely the walk to "the newest outbound after this request",
-- and it carries the kind and provider columns the thread comparisons match on
-- as well. A second index on a narrower prefix would be a duplicate the planner
-- picks between rather than a capability the table gained.
