SET LOCAL lock_timeout = '3s';
-- Somebody a seat wrote to before connecting the mailbox is `mailbox_history`:
-- the company already held that correspondence, so reading it here is not a
-- new acquisition and opens no Art. 14 case (consent.DutyFor). Before this kind
-- existed a mailbox import recorded them as `unknown_legacy` and dated the
-- duty from the oldest mail, so years of history landed on the Focus list as
-- notices overdue since the day they were sent.
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
        'crm_migration'::text,
        'mailbox_history'::text
    ]));

-- Capture's own unknowns are reclassified by the rule capture now applies
-- (contacts.writtenToBeforeConnectedTx, spelled again here because a migration
-- cannot call Go): a mail the provider filed as sent to one of the contact's
-- addresses, sent by the seat whose mailbox delivered it, and dated before that
-- seat's first connection. An unknown a seat or an import stated is a claim
-- about somewhere else and stays.
--
-- Their open duties close as exempt_with_reason with the ground on the row and
-- one audit entry each, the way SettleWhenSubjectWroteTx closes a duty. The
-- cases are locked before they are read, so the audit before-image is the row
-- that was overwritten; a queued case keeps its delivery, and a terminal one
-- keeps its ground.
WITH history AS (
    UPDATE contact_acquisition_evidence e
       SET kind = 'mailbox_history'
     WHERE e.kind = 'unknown_legacy'
       AND starts_with(e.captured_by, 'connector:')
       AND EXISTS (
             SELECT 1 FROM contact_email ce
               JOIN activity a ON a.counterparty_email = ce.email
               JOIN capture_import ci ON ci.activity_id = a.id
               JOIN activity_participant s
                 ON s.activity_id = a.id AND s.role = 'from' AND s.user_id = ci.user_id
              WHERE ce.contact_id = e.contact_id AND ce.archived_at IS NULL
                AND a.kind = 'email' AND a.direction = 'outbound'
                AND a.counterparty_outbound_attested AND a.archived_at IS NULL
                AND a.occurred_at < (SELECT min(cc.created_at) FROM capture_connection cc
                                      WHERE cc.user_id = ci.user_id))
    RETURNING e.id
),
owed AS (
    SELECT c.id, c.state, c.rule, c.owner_user_id
      FROM privacy_notice_case c
     WHERE c.acquisition_id IN (SELECT id FROM history)
       AND c.state IN ('open', 'assigned', 'blocked', 'delivery_failed')
       FOR UPDATE
),
settled AS (
    UPDATE privacy_notice_case c
       SET state = 'exempt_with_reason',
           resolution_note = 'Mailbox history: we wrote to them from a company mailbox before it was connected, so the correspondence was already held and no new notice is owed.',
           completed_at = now(), blocked_reason = NULL, updated_at = now()
      FROM owed
     WHERE c.id = owed.id
       AND c.state IN ('open', 'assigned', 'blocked', 'delivery_failed')
    RETURNING c.id
)
INSERT INTO audit_log (actor_type, actor_id, action, entity_type, entity_id, before, after)
SELECT 'system', 'migration', 'update', 'privacy_notice_case', owed.id,
       jsonb_strip_nulls(jsonb_build_object('state', owed.state, 'owner_user_id', owed.owner_user_id)),
       jsonb_build_object('state', 'exempt_with_reason', 'rule', owed.rule, 'resolution_note', true)
  FROM owed JOIN settled ON settled.id = owed.id;
