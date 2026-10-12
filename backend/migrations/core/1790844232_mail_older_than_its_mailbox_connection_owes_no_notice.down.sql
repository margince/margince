SET LOCAL lock_timeout = '3s';
-- The duties this migration closed reopen, found by the ground it wrote, and
-- their acquisitions fall back to the honest unknown before the narrower check
-- returns.
UPDATE privacy_notice_case
   SET state = 'open', resolution_note = NULL, completed_at = NULL, updated_at = now()
 WHERE state = 'exempt_with_reason'
   AND resolution_note = 'Mailbox history: we wrote to them from a company mailbox before it was connected, so the correspondence was already held and no new notice is owed.';
-- A mailbox_history acquisition written after the upgrade never got a case.
-- As unknown_legacy it owes one, dated the way noticecaseopen.go dates it, so
-- the duty is recorded rather than left owed and invisible.
INSERT INTO privacy_notice_case (contact_id, acquisition_id, rule, due_at, allowed_routes, state)
SELECT e.contact_id, e.id, 'art14',
       coalesce(e.occurred_at, e.captured_at) + interval '1 month',
       ARRAY['privacy_notice', 'record_confirmation'], 'open'
  FROM contact_acquisition_evidence e
 WHERE e.kind = 'mailbox_history'
   AND NOT EXISTS (SELECT 1 FROM privacy_notice_case c WHERE c.acquisition_id = e.id);
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
