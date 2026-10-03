SET LOCAL lock_timeout = '3s';
-- Mail a seat sent from another address of theirs (a former employer's, a
-- second domain) was captured INBOUND with that address as its counterparty,
-- because the connector compared From with the grant address alone. Capture now
-- re-reads such mail as the seat's outbound mail when the provider filed it as
-- sent (capture.asSentFromOwnAddressTx). The provider's filing was never
-- stored, so for mail captured before that, the evidence is:
--   * the activity's captured_by names this seat's own Gmail or Graph
--     connection, and exactly one seat imported it;
--   * its counterparty is an exact address the seat holds: one in
--     capture_owner_identity, or the account its connection was granted for;
--   * the head of the stored original carries no Received header. Every hop
--     that delivers mail prepends one, so a message in the mailbox without any
--     was never delivered to it: it was written there, by its owner, as sent;
--   * it is not bulk mail, and its Date header is before the seat's FIRST
--     connection, as contacts.writtenToBeforeConnectedTx asks of sent mail.
-- A capture-made unknown_legacy acquisition for an address on the To or Cc line
-- of such mail is re-labelled mailbox_history, and its open duties close as
-- exempt_with_reason, each with one audit entry, as 1790871111 does. The
-- activities themselves are left as they were captured.
WITH candidate AS (
    SELECT a.id,
           -- Only the head of each original is read; bytes that are not the
           -- shape capture stores read as no evidence, never as an error.
           CASE
             WHEN jsonb_typeof(rc.payload) = 'string' THEN left(rc.payload #>> '{}', 65536)
             WHEN left(rc.payload ->> 'data', 87380) ~ '^([A-Za-z0-9+/]{4})*([A-Za-z0-9+/]{2}==|[A-Za-z0-9+/]{3}=)?$'
             THEN encode(decode(left(rc.payload ->> 'data', 87380), 'base64'), 'escape')
           END AS text_head
      FROM activity a
      JOIN capture_import ci ON ci.activity_id = a.id
      JOIN raw_capture rc ON rc.id = a.raw_capture_id
     WHERE a.kind = 'email' AND a.direction = 'inbound'
       AND NOT a.bulk_mail_attested AND a.archived_at IS NULL
       AND a.captured_by IN ('connector:gmail:' || ci.user_id::text, 'connector:graph:' || ci.user_id::text)
       AND NOT EXISTS (SELECT 1 FROM capture_import o
                        WHERE o.activity_id = ci.activity_id AND o.user_id <> ci.user_id)
       AND (EXISTS (SELECT 1 FROM capture_owner_identity oi
                     WHERE oi.user_id = ci.user_id AND oi.kind = 'address'
                       AND oi.value = a.counterparty_email)
            OR EXISTS (SELECT 1 FROM capture_connection cc
                        WHERE cc.user_id = ci.user_id AND lower(cc.account_label) = a.counterparty_email))
       AND a.occurred_at < (SELECT min(cc.created_at) FROM capture_connection cc WHERE cc.user_id = ci.user_id)
),
sent AS (
    SELECT id FROM candidate
     WHERE substring(text_head FROM '^(.*?)\r?\n\r?\n') !~* '(^|\n)received:'
),
history AS (
    UPDATE contact_acquisition_evidence e
       SET kind = 'mailbox_history'
     WHERE e.kind = 'unknown_legacy'
       AND (starts_with(e.captured_by, 'connector:') OR e.captured_by = 'agent:capture_counterparty_verdict')
       AND EXISTS (
             SELECT 1 FROM contact_email ce
               JOIN activity_participant p ON p.address = ce.email AND p.role IN ('to', 'cc')
               JOIN sent ON sent.id = p.activity_id
              WHERE ce.contact_id = e.contact_id AND ce.archived_at IS NULL)
    RETURNING e.id
),
reclassified AS (
    INSERT INTO audit_log (actor_type, actor_id, action, entity_type, entity_id, before, after)
    SELECT 'system', 'migration', 'update', 'contact_acquisition_evidence', history.id,
           jsonb_build_object('kind', 'unknown_legacy'),
           jsonb_build_object('kind', 'mailbox_history', 'datafix', '2026-10-03_mail_sent_from_a_former_address_owes_no_notice')
      FROM history
),
owed AS (
    SELECT c.id, c.state, c.rule, c.owner_user_id, c.blocked_reason
      FROM privacy_notice_case c
     WHERE c.acquisition_id IN (SELECT id FROM history)
       AND c.state IN ('open', 'assigned', 'blocked', 'delivery_failed')
       FOR UPDATE
),
settled AS (
    UPDATE privacy_notice_case c
       SET state = 'exempt_with_reason',
           resolution_note = 'Mailbox history: before a company mailbox was connected, it already held mail we sent them or mail we received that was also addressed to them, so the correspondence was already held and no new notice is owed.',
           completed_at = now(), blocked_reason = NULL, updated_at = now()
      FROM owed
     WHERE c.id = owed.id
       AND c.state IN ('open', 'assigned', 'blocked', 'delivery_failed')
    RETURNING c.id
)
INSERT INTO audit_log (actor_type, actor_id, action, entity_type, entity_id, before, after)
SELECT 'system', 'migration', 'update', 'privacy_notice_case', owed.id,
       jsonb_strip_nulls(jsonb_build_object('state', owed.state, 'owner_user_id', owed.owner_user_id,
                                            'blocked_reason', owed.blocked_reason)),
       jsonb_build_object('state', 'exempt_with_reason', 'rule', owed.rule, 'resolution_note', true,
                          'datafix', '2026-10-03_mail_sent_from_a_former_address_owes_no_notice')
  FROM owed JOIN settled ON settled.id = owed.id;
