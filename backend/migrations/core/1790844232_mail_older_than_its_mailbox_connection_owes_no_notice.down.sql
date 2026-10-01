SET LOCAL lock_timeout = '3s';
-- The duties this migration closed reopen, found by the ground it wrote, and
-- their acquisitions fall back to the honest unknown before the narrower check
-- returns.
UPDATE privacy_notice_case
   SET state = 'open', resolution_note = NULL, completed_at = NULL, updated_at = now()
 WHERE state = 'exempt_with_reason'
   AND resolution_note = 'Mailbox history: we wrote to them from a company mailbox before it was connected, so the correspondence was already held and no new notice is owed.';
UPDATE contact_acquisition_evidence SET kind = 'unknown_legacy' WHERE kind = 'mailbox_history';
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
