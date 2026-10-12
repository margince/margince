-- An answer is found by index: the arms that decide whether an inbound message
-- was answered probe only rows that could be an answer.
--
-- answerArms (activities/answered.go) reads, per inbound message, our
-- same-subject mail to the sender and our calls and held meetings with the
-- sender's contact. The response spread the waiting horizon follows runs that
-- per inbound row over a year; in production it took 30 s and 11.3 M buffer
-- hits for 4,513 messages, and timed the owed_verdict pass out.
--
-- idx_activity_answer_touch serves the call / held-meeting arms. They walk
-- every activity linked to (or attended by) the contact — ~120 and ~170 per
-- message in production — and fetched each one through activity_pkey and the
-- heap only to throw it away on kind. Calls and held meetings are about one
-- activity in nine, so the probe becomes an index-only miss in an index a
-- ninth the size, and the heap is touched only for a real answer. The
-- predicate is the arm's own filter clause for clause (the touch and
-- everyoneReads fragments), which is what lets the planner prove it applies;
-- occurred_at rides along so the "strictly later" test needs no heap either.
--
-- idx_activity_answer_mail serves the same-subject mail arm. It looked the
-- sender's address up in idx_activity_counterparty_email and ANDed that bitmap
-- with idx_activity_kind — every live mail in the installation, rebuilt per
-- probe. Attested outbound mail is a few per cent of activity, so keying the
-- address on it answers the arm without the second bitmap.
--
-- Bounded: an open transaction holding a conflicting lock would otherwise stall
-- every write to activity for as long as this migration is willing to queue.
SET LOCAL lock_timeout = '3s';

-- Not CONCURRENTLY: a migration runs in one transaction, which forbids it. Each
-- build holds a write-blocking lock on activity for its duration, which is the
-- same bargain every index in this tree makes — and activity's live rows are
-- tens of thousands, so the duration is a second or two.
CREATE INDEX IF NOT EXISTS idx_activity_answer_touch
    ON activity (id, occurred_at)
    WHERE (kind = 'call' OR (kind = 'meeting' AND meeting_status = 'held'))
      AND restricted_at IS NULL
      AND audience = 'workspace'
      AND archived_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_activity_answer_mail
    ON activity (counterparty_email, kind, occurred_at)
    WHERE direction = 'outbound'
      AND counterparty_outbound_attested
      AND restricted_at IS NULL
      AND audience = 'workspace'
      AND archived_at IS NULL;
