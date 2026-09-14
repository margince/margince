SET LOCAL lock_timeout = '3s';

-- Which message carried a disclosure, and what became of it.
--
-- A notice case could say a disclosure was QUEUED and never which message
-- carried it. That left two things unanswerable and one thing wrong.
--
-- Unanswerable: an auditor asking "you say you told them, show me" had nothing
-- to open, and an operator asking "when was the last one actually sent" had
-- only updated_at — which moves for any write, so a merge relinking the contact
-- looks like a disclosure going out.
--
-- Wrong: a queued disclosure that BOUNCED left the case reading `queued`
-- forever. The duty was not met, the subject was never told, and the queue
-- showed a case somebody had handled. That is the failure a compliance control
-- must not have, because it is invisible: the case looks better than an open
-- one while being worse.
ALTER TABLE privacy_notice_case
    ADD COLUMN delivery_id uuid
        REFERENCES comms_outbound(id) ON DELETE SET NULL;

-- When a disclosure was last SENT for this duty, as distinct from when the row
-- was last touched. The cooldown in noticedischarge.go measures from
-- updated_at because this column did not exist, and says so: a merge touching
-- the contact extends somebody's cooldown by up to a month. With this the
-- window is exact.
ALTER TABLE privacy_notice_case
    ADD COLUMN last_sent_at timestamptz;

-- EXISTING QUEUED CASES ARE REOPENED rather than constrained.
--
-- A case queued before this migration has no delivery_id and never will: the
-- column did not exist when its disclosure went out, and the message that
-- carried it is not identifiable after the fact. Adding the constraint below
-- without this would fail the migration on exactly those rows.
--
-- `open` is the honest landing place, and not merely the convenient one. What
-- `queued` claims is "a message is on its way", which for a mail staged weeks
-- ago is no longer true whatever happened to it — and this installation cannot
-- say whether it arrived. Moving them to `open` puts the duty back in front of
-- somebody, which is the safe direction: the worst case is a second disclosure
-- to a subject who already had one, against a duty nobody ever discharges.
UPDATE privacy_notice_case
   SET state = 'open', updated_at = now()
 WHERE state = 'queued' AND delivery_id IS NULL;

-- A queued case says which message it is waiting on. Without this a row could
-- claim a disclosure is on its way while pointing at nothing, which is the
-- state `queued` alone already had and this change exists to end.
--
-- It constrains `queued` ALONE, not "every state that has ever queued". An
-- `open` case may carry a delivery_id from an earlier attempt, and a
-- delivery_failed one must: that message is the evidence of what went wrong,
-- and dropping the pointer would leave the case saying delivery failed with no
-- way to ask which delivery.
ALTER TABLE privacy_notice_case
    ADD CONSTRAINT privacy_notice_case_queued_names_its_delivery
        CHECK (state <> 'queued' OR delivery_id IS NOT NULL);

-- A NINTH state: a disclosure this installation sent that did not arrive.
--
-- It is UNRESOLVED, which is the whole point. The duty is owed again — the
-- subject was not told — so the case returns to the queue rather than resting
-- in a terminal state that reads like an outcome. `blocked` would be the wrong
-- shape: blocked says we cannot send, and here we did send and it failed, which
-- an operator answers differently (correct the address, then re-send).
ALTER TABLE privacy_notice_case
    DROP CONSTRAINT privacy_notice_case_state;

ALTER TABLE privacy_notice_case
    ADD CONSTRAINT privacy_notice_case_state
        CHECK (state = ANY (ARRAY[
            'open'::text,
            'assigned'::text,
            'queued'::text,
            'delivery_failed'::text,
            'completed'::text,
            'provided_elsewhere'::text,
            'exempt_with_reason'::text,
            'blocked'::text,
            'not_required'::text
        ]));

-- The queue's own read gains the new state, so a failed disclosure is on the
-- lane beside the open duties rather than needing a second query to find.
DROP INDEX IF EXISTS privacy_notice_case_due;

CREATE INDEX privacy_notice_case_due
    ON privacy_notice_case (due_at, id)
    WHERE state IN ('open', 'assigned', 'queued', 'delivery_failed', 'blocked');

-- Finding the case a bounced delivery belongs to. Partial because only a row
-- that named a delivery can be reached this way, and those are a small fraction
-- of the table.
CREATE INDEX privacy_notice_case_delivery
    ON privacy_notice_case (delivery_id)
    WHERE delivery_id IS NOT NULL;
