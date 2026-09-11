SET LOCAL lock_timeout = '3s';

-- A rep refused at the keyboard usually cannot override the engine themselves.
--
-- Directing a send takes an authority most seats do not hold, so the rep's own
-- next move is not "override it" — it is "ask somebody who can". Until now
-- there was nowhere for that ask to go: the review recorded what was refused
-- and sat there, and a rep without the grant could read it and do nothing.
--
-- This is where the ask is recorded. The review moves to a state saying it is
-- somebody else's to answer, and names the approval card carrying the question.

-- The card this review was handed to. Nullable because most reviews are never
-- routed: a rep who holds the authority directs the send themselves, and one
-- who gives up leaves the review where it is.
--
-- NO FOREIGN KEY, deliberately, and this is the one place it was worth being
-- careful about.
--
-- An approval is queue work with its own lifecycle and gets swept. A
-- RESTRICT would make a routed review block that sweep. A SET NULL looks like
-- the answer and is worse: the shape CHECK below requires a routed review to
-- name its card, so nulling the column would violate it and Postgres refuses
-- the delete anyway — the same block, arriving as a constraint error from a
-- sweep that has nothing to do with consent.
--
-- What the column is for is FINDING the card, and a swept card is not there to
-- find. An id pointing at nothing reads exactly as what happened: this was
-- handed on, and the queue has since moved past it.
ALTER TABLE communication_review
    ADD COLUMN approval_id uuid;

-- 'awaiting_decision' joins the states a live review can be in.
--
-- It is a fact about WHO the work belongs to rather than about the message, in
-- the same voice as the rest of the vocabulary: needs_context says what would
-- answer the refusal, and this says the answering is no longer the rep's to do.
ALTER TABLE communication_review
    DROP CONSTRAINT communication_review_state,
    ADD CONSTRAINT communication_review_state CHECK (state = ANY (ARRAY[
        'needs_context'::text, 'needs_repair'::text, 'awaiting_decision'::text,
        'resolved'::text, 'superseded'::text, 'cancelled'::text]));

-- A routed review names the card it was routed to. Without this a review could
-- read as waiting for a decision with nothing carrying the question, which is a
-- rep waiting forever for an answer nobody was asked for.
ALTER TABLE communication_review
    ADD CONSTRAINT communication_review_routing_shape CHECK (
        (state <> 'awaiting_decision'::text) OR (approval_id IS NOT NULL));

-- The reviewer's own view: what has been handed to somebody, oldest first.
CREATE INDEX communication_review_awaiting
    ON communication_review (opened_at)
    WHERE state = 'awaiting_decision';
