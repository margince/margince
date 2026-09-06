-- Bound the wait for the lock below. Without it an open transaction holding a
-- conflicting lock stalls every write to person for as long as this migration
-- is willing to queue, which is forever.
SET LOCAL lock_timeout = '3s';

-- What the installation OWES a person it obtained without asking them.
--
-- person_acquisition_evidence already records how a contact came to exist. It
-- says nothing about the duty that follows: a person acquired from a list, a
-- referral or an import never asked to hear from us, and Art. 14 gives the
-- controller one month to tell them we hold their data and how to object.
-- Today that duty is owed and unrecorded, so nobody can answer "who have we
-- not told yet" — and a duty nobody can enumerate is one nobody discharges.
--
-- ONE CASE PER ACQUISITION, not per person. A contact acquired twice — a form
-- today, an import batch next month — incurs the duty twice, and the second
-- acquisition is a fresh disclosure with its own deadline. Keying on the person
-- would silently collapse them and mark the second discharged by the first.
CREATE TABLE privacy_notice_case (
    id uuid DEFAULT uuidv7() NOT NULL,
    person_id uuid NOT NULL,
    -- The acquisition that incurred the duty. CASCADE because the case is a
    -- statement ABOUT that acquisition: erasure clears the evidence, and a case
    -- pointing at nothing would outlive the only record of why it existed.
    acquisition_id uuid NOT NULL,
    -- Which rule put this here, so a later reader can tell an Art. 14 duty from
    -- an Art. 13 one without re-deriving it from the acquisition kind. The kind
    -- is evidence of what happened; this is the conclusion drawn from it, and
    -- the conclusion is what the deadline hangs off.
    rule text NOT NULL,
    -- The moment the duty falls due. NOT NULL and never derived at read time:
    -- Art. 14's month runs from the acquisition, and an import landing today
    -- carrying last year's business card is already late.
    due_at timestamptz NOT NULL,
    -- How this case may be discharged. A case with no route is one the product
    -- cannot close by sending anything, and it says so rather than offering a
    -- button that fails.
    allowed_routes text[] NOT NULL DEFAULT '{}',
    state text NOT NULL DEFAULT 'open',
    -- Who owes it. Nullable: a case can outlive the seat that created it, and
    -- an unowned overdue case must still appear on the queue rather than
    -- vanishing with its owner.
    owner_user_id uuid,
    attempts integer NOT NULL DEFAULT 0,
    completed_at timestamptz,
    blocked_reason text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT privacy_notice_case_pkey PRIMARY KEY (id),
    CONSTRAINT privacy_notice_case_person_fkey
        FOREIGN KEY (person_id) REFERENCES person(id) ON DELETE CASCADE,
    CONSTRAINT privacy_notice_case_acquisition_fkey
        FOREIGN KEY (acquisition_id) REFERENCES person_acquisition_evidence(id) ON DELETE CASCADE,
    CONSTRAINT privacy_notice_case_owner_fkey
        FOREIGN KEY (owner_user_id) REFERENCES app_user(id) ON DELETE SET NULL,
    -- NO 'overdue' STATE, deliberately. Overdue is a reading of the clock
    -- against due_at, not a fact somebody writes: a stored one would make a
    -- case overdue only once a sweep had run, so a job that never fires would
    -- leave every late case looking on time — silently, which is the one way
    -- this control must not fail. data_subject_request settles it the same way:
    -- no overdue in its status vocabulary, and its due-soonest lane orders by
    -- due_at rather than filtering on one.
    CONSTRAINT privacy_notice_case_state
        CHECK (state = ANY (ARRAY[
            'open'::text,
            'queued'::text,
            'completed'::text,
            'blocked'::text,
            'not_required'::text
        ])),
    CONSTRAINT privacy_notice_case_rule
        CHECK (rule = ANY (ARRAY['art13'::text, 'art14'::text])),
    -- A terminal state says WHEN it became terminal, and a live one has not.
    -- Without this a completed case carrying no timestamp is indistinguishable
    -- from one somebody closed and forgot to date.
    CONSTRAINT privacy_notice_case_completion_shape
        CHECK ((state IN ('completed', 'not_required')) = (completed_at IS NOT NULL)),
    -- Blocked says why. A blocked case with no reason is a dead end nobody can
    -- act on, which is how a duty quietly stops being anybody's.
    CONSTRAINT privacy_notice_case_blocked_shape
        CHECK ((state = 'blocked') = (blocked_reason IS NOT NULL))
);

-- ONE case per acquisition. The consumer of person.created is at-least-once,
-- so a redelivery must not mint a second case for the same duty — the database
-- refuses it rather than the handler remembering to check.
CREATE UNIQUE INDEX privacy_notice_case_acquisition
    ON privacy_notice_case (acquisition_id);

-- The queue's own read: unresolved cases, soonest deadline first. Partial
-- because a completed case is never on it, and the queue is the only reader
-- that matters for how this table is scanned.
CREATE INDEX privacy_notice_case_due
    ON privacy_notice_case (due_at, id)
    WHERE state IN ('open', 'queued');
