SET LOCAL lock_timeout = '3s';

-- A send the engine refused is a piece of work, not an error message.
--
-- Until now a refusal rolled its whole transaction back and answered
-- 409 consent_not_granted. Nothing durable was left: not what was refused, not
-- which recipient it was refused for, not what would change the answer. A rep
-- pressed send, read a code, and had nowhere to go.
--
-- This row is what a refusal leaves behind. It names the recipients and the
-- reason each was refused, and it binds to the held delivery intent so the same
-- message can resume rather than being retyped.
--
-- Tenancy is the database, as everywhere else in this schema: the handle is
-- bound to the workspace it serves, so there is no workspace column here and no
-- row-level policy — the same shape communication_decision and
-- communication_suppression carry.
CREATE TABLE communication_review (
    id uuid PRIMARY KEY DEFAULT uuidv7(),

    -- What state the work is in. The vocabulary is about what is NEEDED rather
    -- than about who is blocked: needs_context is a fact about the message, and
    -- it stays true whoever is looking at it.
    state text NOT NULL DEFAULT 'needs_context',

    -- Which door refused. One review answers one send attempt; a second attempt
    -- at the same intent supersedes rather than duplicating.
    kind text NOT NULL DEFAULT 'single',

    -- The held scheduled_send this review can resume. Nullable because a send
    -- refused at the keyboard may have no intent row yet; the slice that holds
    -- one for every refusal fills this in.
    delivery_intent_id uuid REFERENCES scheduled_send(id) ON DELETE CASCADE,

    -- Who pressed send. Not who may resolve it: that is a permission question
    -- answered at read time, and storing an answer would freeze it.
    initiated_by uuid REFERENCES app_user(id) ON DELETE SET NULL,

    -- WHAT WAS REFUSED, AND WHY, per recipient. jsonb rather than a child table
    -- because it is a SNAPSHOT: it records what the engine said at this moment
    -- and must not change when the underlying consent rows do. A reader asking
    -- "why was this refused on Tuesday" needs Tuesday's answer.
    --
    -- It holds addresses, so the privacy engine clears it with the subject.
    refusals jsonb NOT NULL DEFAULT '[]'::jsonb,

    -- The strongest reason across the recipients, lifted out so a queue can
    -- order and filter without opening the snapshot.
    reason_code text NOT NULL,

    opened_at timestamptz NOT NULL DEFAULT now(),
    resolved_at timestamptz,
    -- Which review replaced this one, when a fresh attempt supersedes it.
    --
    -- ON DELETE RESTRICT, not SET NULL. The CHECK below requires a superseded
    -- review to name its successor, so nulling this column on the successor's
    -- deletion would leave the predecessor violating its own constraint —
    -- a row the database would then refuse every later update to. Refusing the
    -- delete instead is the honest answer: a review that replaced another is
    -- the reason that other one is closed, and removing it would leave the
    -- first claiming a supersession nothing records.
    superseded_by uuid REFERENCES communication_review(id) ON DELETE RESTRICT,

    CONSTRAINT communication_review_state CHECK (state = ANY (ARRAY[
        'needs_context'::text, 'needs_repair'::text,
        'resolved'::text, 'superseded'::text, 'cancelled'::text])),
    CONSTRAINT communication_review_kind CHECK (kind = 'single'::text),
    -- A finished review says when. A live one must not claim a moment.
    CONSTRAINT communication_review_resolution_shape CHECK (
        (state IN ('resolved', 'superseded', 'cancelled')) = (resolved_at IS NOT NULL)),
    -- Only a superseded review names its successor, and it must name one.
    CONSTRAINT communication_review_supersession_shape CHECK (
        (state = 'superseded'::text) = (superseded_by IS NOT NULL))
);

-- ONE LIVE REVIEW PER INTENT. A second attempt at the same held message
-- supersedes the first rather than queueing a duplicate: two live reviews for
-- one message would put the same work in front of somebody twice, and resolving
-- one would leave the other pointing at a message that has already gone.
CREATE UNIQUE INDEX communication_review_one_live_per_intent
    ON communication_review (delivery_intent_id)
    WHERE delivery_intent_id IS NOT NULL AND resolved_at IS NULL;

-- The queue read: open work, oldest first.
CREATE INDEX communication_review_open
    ON communication_review (opened_at)
    WHERE resolved_at IS NULL;

-- The initiator's own view, which is the only one this slice serves.
CREATE INDEX communication_review_by_initiator
    ON communication_review (initiated_by, opened_at DESC);
