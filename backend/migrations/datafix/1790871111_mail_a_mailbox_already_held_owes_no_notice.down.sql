SET LOCAL lock_timeout = '3s';
-- The duties the upgrade closed go back to the state its audit before-image
-- recorded (open, assigned, blocked with its reason, or delivery_failed), with
-- one audit entry each. The latest entry per case wins, so an up, down, up
-- cycle restores what the last upgrade overwrote.
WITH closed AS (
    SELECT DISTINCT ON (l.entity_id) l.entity_id AS id, l.before
      FROM audit_log l
     WHERE l.entity_type = 'privacy_notice_case' AND l.actor_id = 'migration'
       AND l.after ->> 'migration' = '1790871111'
     ORDER BY l.entity_id, l.occurred_at DESC, l.id DESC
),
target AS (
    SELECT c.id, c.state, closed.before
      FROM privacy_notice_case c JOIN closed ON closed.id = c.id
     WHERE c.state = 'exempt_with_reason'
       AND c.resolution_note = 'Mailbox history: before a company mailbox was connected, it already held mail we sent them or mail we received that was also addressed to them, so the correspondence was already held and no new notice is owed.'
       FOR UPDATE OF c
),
reopened AS (
    UPDATE privacy_notice_case c
       SET state = coalesce(target.before ->> 'state', 'open'),
           blocked_reason = target.before ->> 'blocked_reason',
           resolution_note = NULL, completed_at = NULL, updated_at = now()
      FROM target
     WHERE c.id = target.id
    RETURNING c.id, c.state
)
INSERT INTO audit_log (actor_type, actor_id, action, entity_type, entity_id, before, after)
SELECT 'system', 'migration', 'update', 'privacy_notice_case', reopened.id,
       jsonb_build_object('state', 'exempt_with_reason'),
       jsonb_build_object('state', reopened.state, 'migration', '1790871111-down')
  FROM reopened;

-- The acquisitions the upgrade reclassified (each has its audit entry) fall
-- back to the honest unknown, and so does every capture-made mailbox_history
-- acquisition that migration 1790844232's sent-mail rule does not cover: those
-- came from the received-mail rule, which capture also applied after the
-- upgrade. Each gets an audit entry, and each that never had a case gets the
-- duty unknown_legacy owes, dated the way noticecaseopen.go dates it, so the
-- duty is recorded rather than left owed and invisible.
WITH reverted AS (
    UPDATE contact_acquisition_evidence e
       SET kind = 'unknown_legacy'
     WHERE e.kind = 'mailbox_history'
       AND (starts_with(e.captured_by, 'connector:') OR e.captured_by = 'agent:capture_counterparty_verdict')
       AND (EXISTS (
              SELECT 1 FROM audit_log l
               WHERE l.entity_type = 'contact_acquisition_evidence' AND l.entity_id = e.id
                 AND l.actor_id = 'migration' AND l.after ->> 'migration' = '1790871111')
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
),
audited AS (
    INSERT INTO audit_log (actor_type, actor_id, action, entity_type, entity_id, before, after)
    SELECT 'system', 'migration', 'update', 'contact_acquisition_evidence', r.id,
           jsonb_build_object('kind', 'mailbox_history'),
           jsonb_build_object('kind', 'unknown_legacy', 'migration', '1790871111-down')
      FROM reverted r
)
INSERT INTO privacy_notice_case (contact_id, acquisition_id, rule, due_at, allowed_routes, state)
SELECT r.contact_id, r.id, 'art14',
       coalesce(r.occurred_at, r.captured_at) + interval '1 month',
       ARRAY['privacy_notice', 'record_confirmation'], 'open'
  FROM reverted r
 WHERE NOT EXISTS (SELECT 1 FROM privacy_notice_case c WHERE c.acquisition_id = r.id);
