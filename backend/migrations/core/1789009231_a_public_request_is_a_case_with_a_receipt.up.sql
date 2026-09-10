-- A proposal a data subject sends through their confirm link is a request
-- under Art. 16 or Art. 17, and the clock on it starts when it arrives.
--
-- Until now such a proposal landed in person_confirm_submission and stopped
-- there. That table is a record of what was sent, not a piece of work anybody
-- owns: nothing gave it a deadline, nothing put it in the queue the DPO works
-- through, and the subject got no reference to quote when asking after it.
-- The statutory answer was owed either way.

-- person_id, because subject_ref is free text and a case opened from a link
-- knows exactly whose record it is about. FulfilErasure parses subject_ref as
-- a person id, so the two must agree: openRightsCaseTx writes the id into
-- both, and this column is what a later reader joins on without parsing.
-- Bounded, because every statement below takes a lock that blocks writers on
-- data_subject_request, which this migration did not create. An open
-- transaction holding a conflicting lock would otherwise stall every write to
-- the case queue for as long as this migration is willing to queue.
SET LOCAL lock_timeout = '3s';

ALTER TABLE data_subject_request
    ADD COLUMN person_id uuid REFERENCES person(id) ON DELETE SET NULL;

-- When the subject asked, which is not when the row was written. The deadline
-- runs from receipt, and a case opened by a night-time job hours after a
-- morning submission must not quietly gain those hours.
ALTER TABLE data_subject_request
    ADD COLUMN received_at timestamptz;

-- How it reached us. A case the DPO can act on needs to say whether the
-- request came through a mailed link, a phone call or a letter, because the
-- identity checks that follow differ by channel.
ALTER TABLE data_subject_request
    ADD COLUMN channel text;

ALTER TABLE data_subject_request
    ADD CONSTRAINT data_subject_request_channel CHECK (
        channel IS NULL OR channel = ANY (ARRAY[
            'confirm_link'::text, 'email'::text, 'phone'::text,
            'letter'::text, 'in_person'::text, 'operator_ui'::text]));

-- The submission that opened this case, UNIQUE so a replayed submit opens no
-- second case. A replay is the ordinary failure here: a subject who presses
-- submit twice, a mail client that prefetches, a retry after a timeout. Two
-- cases for one request would put the same work in the queue twice and give
-- the subject two references for one answer.
ALTER TABLE data_subject_request
    ADD COLUMN source_submission_id uuid REFERENCES person_confirm_submission(id) ON DELETE SET NULL;

CREATE UNIQUE INDEX data_subject_request_one_case_per_submission
    ON data_subject_request (source_submission_id)
    WHERE source_submission_id IS NOT NULL;

-- What the subject quotes when they ask after it. Short, unambiguous when
-- read aloud over the phone, and not derived from the row id: an id in a
-- receipt is a handle to a database row, and handing one out invites it to be
-- typed back in somewhere that trusts it.
ALTER TABLE data_subject_request
    ADD COLUMN receipt_reference text;

CREATE UNIQUE INDEX data_subject_request_receipt_reference
    ON data_subject_request (receipt_reference)
    WHERE receipt_reference IS NOT NULL;
