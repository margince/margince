SET LOCAL lock_timeout = '3s';

-- A failed disclosure has no representation in the old vocabulary, and the
-- honest answer going back is `open`: the duty is owed, nobody has met it, and
-- that is what an installation on the old states would have shown. The fact
-- that a message was sent and died is lost with the column.
--
-- The shape constraint goes FIRST, because clearing delivery_id while a queued
-- row still requires one would violate it on exactly the rows this is for.
ALTER TABLE privacy_notice_case
    DROP CONSTRAINT privacy_notice_case_queued_names_its_delivery;

UPDATE privacy_notice_case
   SET state = 'open'
 WHERE state = 'delivery_failed';

DROP INDEX IF EXISTS privacy_notice_case_delivery;

DROP INDEX IF EXISTS privacy_notice_case_due;

CREATE INDEX privacy_notice_case_due
    ON privacy_notice_case (due_at, id)
    WHERE state IN ('open', 'queued');

ALTER TABLE privacy_notice_case
    DROP COLUMN last_sent_at;

ALTER TABLE privacy_notice_case
    DROP COLUMN delivery_id;

ALTER TABLE privacy_notice_case
    DROP CONSTRAINT privacy_notice_case_state;

ALTER TABLE privacy_notice_case
    ADD CONSTRAINT privacy_notice_case_state
        CHECK (state = ANY (ARRAY[
            'open'::text,
            'assigned'::text,
            'queued'::text,
            'completed'::text,
            'provided_elsewhere'::text,
            'exempt_with_reason'::text,
            'blocked'::text,
            'not_required'::text
        ]));
