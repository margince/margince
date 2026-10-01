SET LOCAL lock_timeout = '3s';
-- Rows written under the new kind fall back to the honest unknown before the
-- narrower check returns.
UPDATE contact_acquisition_evidence SET kind = 'unknown_legacy' WHERE kind = 'crm_migration';
ALTER TABLE contact_acquisition_evidence DROP CONSTRAINT contact_acquisition_evidence_kind;
ALTER TABLE contact_acquisition_evidence ADD CONSTRAINT contact_acquisition_evidence_kind
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
    ]));
