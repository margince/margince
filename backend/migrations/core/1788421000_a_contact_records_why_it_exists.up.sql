-- Why this contact is in the CRM at all, recorded when the row is created.
--
-- person.source already says WHERE a record came from — "manual", "capture",
-- an import. That answers provenance and is routinely mistaken for permission:
-- a contact typed in by a rep and a contact who filled in a form both end up as
-- rows, and nothing on either says which of them asked to hear from us.
--
-- Acquisition evidence is the other question. It records what the person DID,
-- or what was done to obtain them, in a closed vocabulary — and it is
-- deliberately not a lawful basis. It is the fact a basis would later be argued
-- from, which is why the kinds describe events rather than conclusions.

SET LOCAL lock_timeout = '3s';

CREATE TABLE person_acquisition_evidence (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    person_id uuid NOT NULL REFERENCES person(id) ON DELETE CASCADE,
    kind text NOT NULL,
    -- What the kind points at: the form submission, the inbound message, the
    -- import batch. Typed and id'd rather than described, so a later reader can
    -- go and look rather than trusting a sentence.
    source_entity_type text,
    source_entity_id uuid,
    -- What the acquiring surface said it was for, in its own words. Evidence of
    -- what was claimed at the time, never a permission.
    purpose_claimed text,
    -- When the ACT happened, which is not when the row was written: an import
    -- lands today carrying a business card collected last year.
    occurred_at timestamptz,
    note text,
    captured_by text NOT NULL,
    captured_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT person_acquisition_evidence_kind
        CHECK (kind = ANY (ARRAY[
            'subject_initiated'::text,
            'customer_contract'::text,
            'requested_quote_or_meeting'::text,
            'in_person_permission'::text,
            'referral'::text,
            'event_or_form'::text,
            'public_or_business_source'::text,
            'purchased_or_imported'::text,
            'unknown_legacy'::text
        ])),
    -- Both or neither. A type naming no row describes nothing, and an id with
    -- no type cannot be looked up.
    CONSTRAINT person_acquisition_evidence_source_shape
        CHECK ((source_entity_type IS NULL) = (source_entity_id IS NULL))
);

CREATE INDEX person_acquisition_evidence_by_person
    ON person_acquisition_evidence (person_id, captured_at DESC);
