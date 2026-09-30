SET LOCAL lock_timeout = '3s';
-- A contact migrated from the CRM the installation used before is recorded as
-- that: `crm_migration`. The old system held it under its own notice duty, so
-- moving it here opens no Art. 14 case (consent.DutyFor). Before this kind
-- existed every imported contact was `unknown_legacy`, and a portal import put
-- one overdue-in-a-month disclosure per contact on the Focus list.
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
        'unknown_legacy'::text,
        'crm_migration'::text
    ]));
