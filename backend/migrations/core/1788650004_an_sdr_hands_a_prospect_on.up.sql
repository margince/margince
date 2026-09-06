-- The SDR-to-AE handoff, as a record rather than an event nobody wrote down.
--
-- An SDR qualifies a prospect and hands it to an account executive. Nothing
-- recorded that it happened, so "why were my handoffs rejected" had no data
-- behind it — not a hard question to answer, a question with nothing to answer
-- it from.
--
-- NOT THE SAME HANDOFF the agent surface already means. prepare_handoff there
-- briefs a DELIVERY team on a project that was sold; this is a SALES handoff of
-- a prospect that has not been. Two different objects, and naming this one
-- `sdr_handoff` rather than `handoff` is what keeps them tellable apart in a
-- grep six months from now.
--
-- The rejection REASON is administered, not free text. A reason nobody can
-- count is a reason nobody acts on, and the workspace picks its own list the
-- way it already does for lead disqualification — same shape as
-- lead_disqualify_reason beside it, including the system flag that marks a
-- seeded row an operator may deactivate but not delete out from under history.
SET LOCAL lock_timeout = '3s';

-- The administered reason catalog. Modelled on lead_disqualify_reason, whose
-- shape this repeats deliberately: an operator configuring one of these has
-- already learned the other.
CREATE TABLE IF NOT EXISTS sdr_handoff_reason (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    label text NOT NULL,
    -- Which transition the reason explains. A reason for refusing a handoff and
    -- a reason for sending it back to the queue are different lists, and one
    -- shared list would offer an AE "wrong territory" as a way to recycle.
    applies_to text NOT NULL,
    sort_order integer NOT NULL DEFAULT 0,
    active boolean NOT NULL DEFAULT true,
    -- A seeded row. An operator may deactivate one, never delete it: a rejected
    -- handoff from last quarter still points at the reason it was rejected for,
    -- and removing the row would leave that record explaining nothing.
    system boolean NOT NULL DEFAULT false,
    version bigint NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT sdr_handoff_reason_label_present CHECK (length(btrim(label)) > 0),
    CONSTRAINT sdr_handoff_reason_applies_to CHECK (applies_to IN ('rejected', 'recycled'))
);

-- The handoff itself.
--
-- ONE ROW PER HANDOFF, carrying its CURRENT state, with the transitions kept
-- beside it in sdr_handoff_event. The pair is the same shape deal + deal_stage_history
-- already uses: every read stays one row, and how it got there is recoverable.
CREATE TABLE IF NOT EXISTS sdr_handoff (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    -- What was handed on. A handoff starts from a lead or from a person — an
    -- SDR working an inbound list has leads, one working an account has people —
    -- and exactly one of them is the subject. The CHECK is what stops a row
    -- claiming both or neither.
    lead_id uuid REFERENCES lead(id) ON DELETE CASCADE,
    person_id uuid REFERENCES person(id) ON DELETE CASCADE,
    organization_id uuid REFERENCES organization(id) ON DELETE SET NULL,
    -- Who handed it on, and to whom. Both are seats, not contacts.
    submitted_by uuid NOT NULL REFERENCES app_user(id),
    -- NULLABLE, because a handoff may be offered to a team rather than a named
    -- AE. Whoever accepts becomes the owner; until then nobody is.
    assigned_to uuid REFERENCES app_user(id),
    status text NOT NULL DEFAULT 'submitted',
    -- Why it was refused or sent back, from the administered catalog. NULL while
    -- the handoff is open, and required the moment it is not — the CHECK below
    -- is what makes "rejected with no reason" unrepresentable rather than
    -- merely discouraged.
    reason_id uuid REFERENCES sdr_handoff_reason(id),
    -- What the decider wanted to say beyond the reason. Secondary by design: the
    -- countable answer is reason_id, and this is the sentence a human adds.
    note text,
    -- The deal the acceptance created or linked. NULL until accepted; the
    -- anchor #4492's held-meeting conversion reads rather than guessing from a
    -- deal that merely shares a company.
    deal_id uuid REFERENCES deal(id) ON DELETE SET NULL,
    submitted_at timestamptz NOT NULL DEFAULT now(),
    decided_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    version bigint NOT NULL DEFAULT 1,
    captured_by text NOT NULL,
    CONSTRAINT sdr_handoff_status CHECK (status IN ('submitted', 'accepted', 'rejected', 'recycled')),
    -- Exactly one subject.
    CONSTRAINT sdr_handoff_one_subject CHECK ((lead_id IS NULL) <> (person_id IS NULL)),
    -- A refusal names its reason. Both terminal-with-reason states demand one;
    -- an open or accepted handoff must not carry one, because a reason on an
    -- accepted handoff would describe a refusal that never happened.
    CONSTRAINT sdr_handoff_reason_when_refused CHECK (
        (status IN ('rejected', 'recycled')) = (reason_id IS NOT NULL)),
    -- A decided handoff knows when. Same both-or-neither shape the reply verdict
    -- uses one table over: a moment with no decision claims one that was never
    -- made, and a decision with no moment cannot be aged or ordered.
    CONSTRAINT sdr_handoff_decided_stamped CHECK (
        (status = 'submitted') = (decided_at IS NULL)),
    -- A deal belongs to an acceptance. Attaching one to a rejected handoff would
    -- credit an SDR for a deal their handoff was refused for.
    CONSTRAINT sdr_handoff_deal_when_accepted CHECK (
        deal_id IS NULL OR status = 'accepted')
);

-- The transitions, append-only.
--
-- The handoff row above says where it stands; this says how it got there. A
-- handoff recycled once and accepted later is a different story from one
-- accepted first time, and a status column alone cannot tell them apart — which
-- is the whole of what "why were my handoffs rejected" is asking.
CREATE TABLE IF NOT EXISTS sdr_handoff_event (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    handoff_id uuid NOT NULL REFERENCES sdr_handoff(id) ON DELETE CASCADE,
    -- The state this event moved it INTO. from_status is deliberately absent:
    -- it is the previous event's to_status, and storing it twice is how two
    -- copies of one history come to disagree.
    to_status text NOT NULL,
    reason_id uuid REFERENCES sdr_handoff_reason(id),
    note text,
    actor text NOT NULL,
    occurred_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT sdr_handoff_event_status CHECK (
        to_status IN ('submitted', 'accepted', 'rejected', 'recycled'))
);

-- An SDR's own handoffs, newest first: the read behind "what happened to what I
-- handed on".
CREATE INDEX IF NOT EXISTS idx_sdr_handoff_submitter
    ON sdr_handoff (submitted_by, submitted_at DESC);

-- The AE's inbox: what is waiting on them, oldest first, because a queue is
-- worked from the front.
CREATE INDEX IF NOT EXISTS idx_sdr_handoff_awaiting
    ON sdr_handoff (assigned_to, submitted_at)
    WHERE status = 'submitted';

-- The subject's own handoffs, for a Person 360 that has to say this prospect was
-- handed on and refused.
CREATE INDEX IF NOT EXISTS idx_sdr_handoff_lead ON sdr_handoff (lead_id) WHERE lead_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_sdr_handoff_person ON sdr_handoff (person_id) WHERE person_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_sdr_handoff_event_handoff
    ON sdr_handoff_event (handoff_id, occurred_at DESC);

COMMENT ON TABLE sdr_handoff IS
    'One SDR-to-AE handoff of a prospect, carrying its current state. Not the delivery briefing the agent surface calls a handoff.';
COMMENT ON TABLE sdr_handoff_event IS
    'Append-only record of how a handoff reached its current state, including every rejection and recycle along the way.';
COMMENT ON COLUMN sdr_handoff.deal_id IS
    'The deal an acceptance created or linked. The anchor held-meeting conversion reads, so credit rests on the acceptance rather than on a deal that merely shares a company.';

-- The seeded reasons. Enough to be usable on day one, marked system so an
-- operator may deactivate but not delete them, and short enough that a workspace
-- adding its own is the expected next step rather than a workaround.
INSERT INTO sdr_handoff_reason (label, applies_to, sort_order, system) VALUES
    ('Not qualified', 'rejected', 10, true),
    ('Wrong territory or segment', 'rejected', 20, true),
    ('Already worked by someone else', 'rejected', 30, true),
    ('Not enough information to act on', 'rejected', 40, true),
    ('Timing is wrong, worth another try later', 'recycled', 10, true),
    ('Needs more qualification first', 'recycled', 20, true)
ON CONFLICT DO NOTHING;
