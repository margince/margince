-- Every outbound message records WHY it was permitted, per recipient.
--
-- Until now one caller-supplied string answered the whole question. Naming the
-- seeded `transactional` purpose returned an unconditional allow, so a cold
-- pitch that called itself operational passed every check in the system, and
-- nothing on the delivery said which rule had been applied or what evidence
-- stood behind it. These three tables are the record that was missing.
--
-- All three are consent's. They hold subject data and are reached by Art. 17
-- erasure; none carries a statutory retention class, because the closed
-- vocabulary has two members and neither describes an authorization decision.

-- communication_decision is the immutable answer for ONE recipient at ONE
-- phase of ONE delivery. Staging and transmit are separate rows because they
-- are separate facts: consent can be withdrawn between them, and a row per
-- attempt is what makes "was this still allowed when it actually went?"
-- answerable rather than assumed.
CREATE TABLE communication_decision (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    delivery_id uuid NOT NULL REFERENCES comms_outbound(id) ON DELETE CASCADE,
    -- The dispatcher's attempt counter, which is the freshness clock: a
    -- transmit decision is current only for the attempt that wrote it.
    attempt integer NOT NULL DEFAULT 0,
    decision_set_id uuid NOT NULL,
    recipient_address text NOT NULL,
    subject_kind text,
    subject_id uuid,
    phase text NOT NULL,
    requested_category text,
    resolved_category text NOT NULL,
    verdict text NOT NULL,
    reason_code text NOT NULL,
    basis text,
    -- Constrained like every sibling in this file. A lawful basis is a closed
    -- vocabulary, and an unconstrained column would let a writer store any
    -- string and have the row read back as authorization.
    evidence jsonb NOT NULL DEFAULT '{}'::jsonb,
    suppression text,
    -- sha256 of subject+body. The fingerprint, never the message: a decision
    -- must not become a second copy of what was written.
    content_fingerprint bytea,
    -- What the old purpose gate said, so a disagreement is readable in the row
    -- and not only in a counter that resets when the process does.
    legacy_verdict text,
    mode text NOT NULL,
    actor text NOT NULL,
    decided_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT communication_decision_phase
        CHECK (phase = ANY (ARRAY['staging'::text, 'transmit'::text])),
    CONSTRAINT communication_decision_verdict
        CHECK (verdict = ANY (ARRAY['allow'::text, 'deny'::text, 'review'::text])),
    CONSTRAINT communication_decision_mode
        CHECK (mode = ANY (ARRAY['observe'::text, 'warn'::text, 'enforce'::text])),
    CONSTRAINT communication_decision_subject_shape
        CHECK ((subject_kind IS NULL) = (subject_id IS NULL)),
    CONSTRAINT communication_decision_subject_kind
        CHECK (subject_kind IS NULL OR subject_kind = ANY (ARRAY['person'::text, 'lead'::text])),
    CONSTRAINT communication_decision_basis
        CHECK (basis IS NULL OR basis = ANY (ARRAY[
            'contract'::text, 'precontract_request'::text,
            'subject_initiated_correspondence'::text, 'legitimate_interests'::text,
            'legal_obligation'::text, 'vital_or_security_interest'::text,
            'consent'::text, 'existing_customer_exception'::text,
            'vn_subject_agreement'::text
        ])),
    CONSTRAINT communication_decision_category
        CHECK (resolved_category = ANY (ARRAY[
            'reply_to_inbound'::text, 'requested_followup'::text,
            'precontract_quote'::text, 'active_deal_followup'::text,
            'customer_service'::text, 'account_notice'::text,
            'contract_notice'::text, 'invoice_or_payment'::text,
            'security_notice'::text, 'privacy_notice'::text,
            'record_confirmation'::text, 'consent_confirmation'::text,
            'optout_confirmation'::text, 'marketing'::text
        ]))
);

-- One authoritative row per recipient per DECISION, not per attempt.
--
-- Keying on the attempt looks right and is not: a pacing policy that postpones
-- a delivery hands the attempt number back (RecordDeferral decrements it), so
-- the next dispatch re-authorizes under the same number. A uniqueness key on
-- attempt would make that second, fresher decision collide with the first and
-- be silently dropped — leaving the record asserting that an older verdict,
-- taken before somebody objected, is what permitted the send.
--
-- The decision set is minted per evaluation, so it separates two answers about
-- the same attempt while still refusing a duplicate row inside one evaluation.
CREATE UNIQUE INDEX communication_decision_one_per_decision
    ON communication_decision (decision_set_id, recipient_address, phase);
CREATE INDEX communication_decision_by_subject
    ON communication_decision (subject_id) WHERE subject_id IS NOT NULL;
CREATE INDEX communication_decision_disagreements
    ON communication_decision (decided_at)
    WHERE legacy_verdict IS NOT NULL AND legacy_verdict <> verdict;

-- communication_basis is a durable non-consent basis: the thing that happened,
-- scoped and dated. It replaces reading a bare inbound activity as permanent
-- person-wide permission — an inbound from 2019 is not authority today, and a
-- basis that cannot expire is not a basis, it is an assumption.
CREATE TABLE communication_basis (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    person_id uuid REFERENCES person(id) ON DELETE CASCADE,
    lead_id uuid REFERENCES lead(id) ON DELETE CASCADE,
    kind text NOT NULL,
    -- The thread this basis is scoped to, when it is scoped to one. A reply
    -- basis that named no thread would authorize any message to that person.
    thread_key text,
    source_activity_id uuid REFERENCES activity(id) ON DELETE SET NULL,
    valid_from timestamptz NOT NULL DEFAULT now(),
    valid_until timestamptz,
    note text,
    captured_by text NOT NULL,
    captured_at timestamptz NOT NULL DEFAULT now(),
    revoked_at timestamptz,
    CONSTRAINT communication_basis_one_subject
        CHECK ((person_id IS NULL) <> (lead_id IS NULL)),
    CONSTRAINT communication_basis_window
        CHECK (valid_until IS NULL OR valid_until > valid_from),
    CONSTRAINT communication_basis_kind
        CHECK (kind = ANY (ARRAY[
            'contract'::text, 'precontract_request'::text,
            'subject_initiated_correspondence'::text, 'legitimate_interests'::text,
            'legal_obligation'::text, 'vital_or_security_interest'::text,
            'existing_customer_exception'::text, 'vn_subject_agreement'::text
        ]))
);
CREATE INDEX communication_basis_live_person
    ON communication_basis (person_id, kind)
    WHERE person_id IS NOT NULL AND revoked_at IS NULL;
CREATE INDEX communication_basis_live_lead
    ON communication_basis (lead_id, kind)
    WHERE lead_id IS NOT NULL AND revoked_at IS NULL;

-- communication_suppression is independent of consent, and that independence is
-- the point. A marketing objection is not the absence of a grant: it outranks
-- one, it does not expire on its own, and re-granting must not silently erase
-- it. An address-scoped row also covers a hard bounce, which is a fact about
-- the mailbox rather than about the person's wishes.
CREATE TABLE communication_suppression (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    person_id uuid REFERENCES person(id) ON DELETE CASCADE,
    lead_id uuid REFERENCES lead(id) ON DELETE CASCADE,
    address text,
    kind text NOT NULL,
    source text NOT NULL,
    recorded_at timestamptz NOT NULL DEFAULT now(),
    captured_by text NOT NULL,
    revoked_at timestamptz,
    CONSTRAINT communication_suppression_names_a_target
        CHECK (person_id IS NOT NULL OR lead_id IS NOT NULL OR address IS NOT NULL),
    CONSTRAINT communication_suppression_kind
        CHECK (kind = ANY (ARRAY[
            'marketing_objection'::text,
            'processing_restriction'::text,
            'hard_bounce'::text,
            'subject_request'::text
        ]))
);
CREATE INDEX communication_suppression_live_person
    ON communication_suppression (person_id) WHERE person_id IS NOT NULL AND revoked_at IS NULL;
CREATE INDEX communication_suppression_live_address
    ON communication_suppression (lower(address)) WHERE address IS NOT NULL AND revoked_at IS NULL;
