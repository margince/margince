SET LOCAL lock_timeout = '3s';
-- Capture-made mailbox_history acquisitions that only this upgrade's rules
-- excuse fall back to the honest unknown: the ones whose duty it closed, and
-- the ones migration 1790844232's sent-mail rule does not cover (the
-- received-mail rule, which capture also applied after the upgrade). Each that
-- never had a case gets the duty unknown_legacy owes, dated the way
-- noticecaseopen.go dates it, so the duty is recorded rather than left owed and
-- invisible. Runs before the reopen below, which clears the note it reads.
WITH reverted AS (
    UPDATE contact_acquisition_evidence e
       SET kind = 'unknown_legacy'
     WHERE e.kind = 'mailbox_history'
       AND (starts_with(e.captured_by, 'connector:') OR e.captured_by = 'agent:capture_counterparty_verdict')
       AND (EXISTS (
              SELECT 1 FROM privacy_notice_case c
               WHERE c.acquisition_id = e.id AND c.state = 'exempt_with_reason'
                 AND c.resolution_note = 'Mailbox history: before a company mailbox was connected, it already held mail we sent them or mail we received that was also addressed to them, so the correspondence was already held and no new notice is owed.')
         OR NOT EXISTS (
              SELECT 1 FROM contact_email ce
                JOIN activity a ON a.counterparty_email = ce.email
                JOIN capture_import ci ON ci.activity_id = a.id
                JOIN activity_participant s
                  ON s.activity_id = a.id AND s.role = 'from' AND s.user_id = ci.user_id
               WHERE ce.contact_id = e.contact_id AND ce.archived_at IS NULL
                 AND a.kind = 'email' AND a.direction = 'outbound'
                 AND a.counterparty_outbound_attested AND a.archived_at IS NULL
                 AND a.occurred_at < (SELECT min(cc.created_at) FROM capture_connection cc
                                       WHERE cc.user_id = ci.user_id)))
    RETURNING e.id, e.contact_id, e.occurred_at, e.captured_at
)
INSERT INTO privacy_notice_case (contact_id, acquisition_id, rule, due_at, allowed_routes, state)
SELECT r.contact_id, r.id, 'art14',
       coalesce(r.occurred_at, r.captured_at) + interval '1 month',
       ARRAY['privacy_notice', 'record_confirmation'], 'open'
  FROM reverted r
 WHERE NOT EXISTS (SELECT 1 FROM privacy_notice_case c WHERE c.acquisition_id = r.id);

UPDATE privacy_notice_case
   SET state = 'open', resolution_note = NULL, completed_at = NULL, updated_at = now()
 WHERE state = 'exempt_with_reason'
   AND resolution_note = 'Mailbox history: before a company mailbox was connected, it already held mail we sent them or mail we received that was also addressed to them, so the correspondence was already held and no new notice is owed.';

ALTER TABLE capture_import DROP COLUMN IF EXISTS provider_received_at;
