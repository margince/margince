SET LOCAL lock_timeout = '3s';
-- A contact capture made on its own and a verdict then withdrew (a personal
-- thread, a noise sender) was archived with its notice duty left open, so the
-- worklist kept a deadline for a record nobody processes. Retraction now ends
-- the duty in the same transaction (consent.SettleWhenCaptureWithdrewTx); this
-- ends the ones left open before that.
--
-- A withdrawn contact is one that is archived, was made by a connector or an
-- agent, and whose CURRENT archive the audit log attributes to one of the two
-- verdict agents that retract (contacts.RetractCaptureOnlyContactTx): its
-- latest archive or restore entry. A verdict's archive that was restored and
-- archived again by a human is the human's, and its duties stay as they are.
WITH withdrawn AS (
    SELECT c.id
      FROM contact c
     WHERE c.archived_at IS NOT NULL
       AND (starts_with(c.captured_by, 'connector:') OR starts_with(c.captured_by, 'agent:'))
       AND (SELECT a.actor_id FROM audit_log a
             WHERE a.entity_type = 'contact' AND a.entity_id = c.id AND a.action IN ('archive', 'restore')
             ORDER BY a.occurred_at DESC, a.id DESC LIMIT 1)
           IN ('agent:capture_confidentiality_verdict', 'agent:capture_counterparty_verdict')
),
owed AS (
    SELECT n.id, n.state, n.rule, n.owner_user_id, n.blocked_reason
      FROM privacy_notice_case n
     WHERE n.contact_id IN (SELECT id FROM withdrawn)
       AND n.state IN ('open', 'assigned', 'blocked', 'delivery_failed')
       FOR UPDATE
),
settled AS (
    UPDATE privacy_notice_case n
       SET state = 'exempt_with_reason',
           resolution_note = 'Withdrawn by capture: the record was made from mail on its own, a verdict then judged that mail private or noise, and the record was archived rather than processed as business data, so no notice is owed.',
           completed_at = now(), blocked_reason = NULL, updated_at = now()
      FROM owed
     WHERE n.id = owed.id
       AND n.state IN ('open', 'assigned', 'blocked', 'delivery_failed')
    RETURNING n.id
)
INSERT INTO audit_log (actor_type, actor_id, action, entity_type, entity_id, before, after)
SELECT 'system', 'migration', 'update', 'privacy_notice_case', owed.id,
       jsonb_strip_nulls(jsonb_build_object('state', owed.state, 'owner_user_id', owed.owner_user_id,
                                            'blocked_reason', owed.blocked_reason)),
       jsonb_build_object('state', 'exempt_with_reason', 'rule', owed.rule, 'resolution_note', true,
                          'datafix', '2026-10-03-3_a_contact_capture_withdrew_owes_no_notice')
  FROM owed JOIN settled ON settled.id = owed.id;
