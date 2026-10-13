SET LOCAL lock_timeout = '3s';
-- Two rules settle a capture-made notice duty from mail already captured, and
-- each now reaches cases it used to miss. This settles the ones opened before.
-- "Capture-made" is an unknown_legacy acquisition written by a connector or by
-- the counterparty verdict, which mints contacts from captured mail afterwards;
-- an unknown a seat or an import stated is a claim about somewhere else and
-- stays.

-- 1. They wrote to us (compose.settleWhenSubjectWrote). The settler read only
--    connector-written acquisitions, so a verdict-made contact who wrote to us
--    kept the duty. As the settler does, the contact gains a subject_initiated
--    acquisition naming their earliest such mail, and the capture-made duties
--    close.
WITH wrote AS (
    SELECT DISTINCT ON (e.contact_id) e.contact_id, a.id AS mail, a.occurred_at
      FROM contact_acquisition_evidence e
      JOIN privacy_notice_case n ON n.acquisition_id = e.id
      JOIN contact_email ce ON ce.contact_id = e.contact_id AND ce.archived_at IS NULL
      JOIN activity_participant p ON lower(p.address) = ce.email AND p.address IS NOT NULL AND p.role = 'from'
      JOIN activity a ON a.id = p.activity_id
     WHERE e.kind = 'unknown_legacy'
       AND (starts_with(e.captured_by, 'connector:') OR e.captured_by = 'agent:capture_counterparty_verdict')
       AND n.state IN ('open', 'assigned', 'blocked', 'delivery_failed')
       AND a.kind = 'email' AND a.direction = 'inbound'
       AND a.archived_at IS NULL AND NOT a.bulk_mail_attested
       AND starts_with(a.captured_by, 'connector:')
     ORDER BY e.contact_id, a.occurred_at, a.id
),
recorded AS (
    INSERT INTO contact_acquisition_evidence
           (contact_id, kind, source_entity_type, source_entity_id, occurred_at, captured_by)
    SELECT w.contact_id, 'subject_initiated', 'activity', w.mail, w.occurred_at, 'system:notice-case-open'
      FROM wrote w
     WHERE NOT EXISTS (SELECT 1 FROM contact_acquisition_evidence x
                        WHERE x.contact_id = w.contact_id AND x.kind = 'subject_initiated')
),
owed AS (
    SELECT n.id, n.state, n.rule, n.owner_user_id, n.blocked_reason
      FROM privacy_notice_case n
      JOIN contact_acquisition_evidence e ON e.id = n.acquisition_id
     WHERE e.contact_id IN (SELECT contact_id FROM wrote)
       AND e.kind = 'unknown_legacy'
       AND (starts_with(e.captured_by, 'connector:') OR e.captured_by = 'agent:capture_counterparty_verdict')
       AND n.state IN ('open', 'assigned', 'blocked', 'delivery_failed')
       FOR UPDATE OF n
),
settled AS (
    UPDATE privacy_notice_case n
       SET state = 'exempt_with_reason',
           resolution_note = 'They wrote to us: a captured mail from their own address shows the data came from them (Art. 13, not Art. 14), so no separate notice is owed.',
           completed_at = now(), blocked_reason = NULL, updated_at = now()
      FROM owed
     WHERE n.id = owed.id
    RETURNING n.id
)
INSERT INTO audit_log (actor_type, actor_id, action, entity_type, entity_id, before, after)
SELECT 'system', 'migration', 'update', 'privacy_notice_case', owed.id,
       jsonb_strip_nulls(jsonb_build_object('state', owed.state, 'owner_user_id', owed.owner_user_id,
                                            'blocked_reason', owed.blocked_reason)),
       jsonb_build_object('state', 'exempt_with_reason', 'rule', owed.rule, 'resolution_note', true,
                          'datafix', '2026-10-03-4_open_duties_the_captured_mail_already_answers')
  FROM owed JOIN settled ON settled.id = owed.id;

-- 2. We wrote to them before connecting (contacts.writtenToBeforeConnectedTx),
--    which now counts the To and Cc lines of that mail and not only its
--    counterparty. The acquisition becomes mailbox_history, as 1790871111 does
--    for received mail, and its owed duties close.
WITH history AS (
    UPDATE contact_acquisition_evidence e
       SET kind = 'mailbox_history'
     WHERE e.kind = 'unknown_legacy'
       AND (starts_with(e.captured_by, 'connector:') OR e.captured_by = 'agent:capture_counterparty_verdict')
       AND EXISTS (
             SELECT 1 FROM contact_email ce
               JOIN activity a ON a.kind = 'email' AND a.direction = 'outbound'
                              AND a.counterparty_outbound_attested AND a.archived_at IS NULL
               JOIN capture_import ci ON ci.activity_id = a.id
               JOIN activity_participant s
                 ON s.activity_id = a.id AND s.role = 'from' AND s.user_id = ci.user_id
              WHERE ce.contact_id = e.contact_id AND ce.archived_at IS NULL
                AND (a.counterparty_email = ce.email
                     OR EXISTS (SELECT 1 FROM activity_participant r
                                 WHERE r.activity_id = a.id AND r.role IN ('to', 'cc') AND r.address = ce.email))
                AND a.occurred_at < (SELECT min(cc.created_at) FROM capture_connection cc
                                      WHERE cc.user_id = ci.user_id))
    RETURNING e.id
),
reclassified AS (
    INSERT INTO audit_log (actor_type, actor_id, action, entity_type, entity_id, before, after)
    SELECT 'system', 'migration', 'update', 'contact_acquisition_evidence', history.id,
           jsonb_build_object('kind', 'unknown_legacy'),
           jsonb_build_object('kind', 'mailbox_history',
                              'datafix', '2026-10-03-4_open_duties_the_captured_mail_already_answers')
      FROM history
),
owed AS (
    SELECT n.id, n.state, n.rule, n.owner_user_id, n.blocked_reason
      FROM privacy_notice_case n
     WHERE n.acquisition_id IN (SELECT id FROM history)
       AND n.state IN ('open', 'assigned', 'blocked', 'delivery_failed')
       FOR UPDATE
),
settled AS (
    UPDATE privacy_notice_case n
       SET state = 'exempt_with_reason',
           resolution_note = 'Mailbox history: before a company mailbox was connected, it already held mail we sent them or mail we received that was also addressed to them, so the correspondence was already held and no new notice is owed.',
           completed_at = now(), blocked_reason = NULL, updated_at = now()
      FROM owed
     WHERE n.id = owed.id
    RETURNING n.id
)
INSERT INTO audit_log (actor_type, actor_id, action, entity_type, entity_id, before, after)
SELECT 'system', 'migration', 'update', 'privacy_notice_case', owed.id,
       jsonb_strip_nulls(jsonb_build_object('state', owed.state, 'owner_user_id', owed.owner_user_id,
                                            'blocked_reason', owed.blocked_reason)),
       jsonb_build_object('state', 'exempt_with_reason', 'rule', owed.rule, 'resolution_note', true,
                          'datafix', '2026-10-03-4_open_duties_the_captured_mail_already_answers')
  FROM owed JOIN settled ON settled.id = owed.id;
