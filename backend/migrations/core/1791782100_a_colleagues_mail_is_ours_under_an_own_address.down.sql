-- The rows the up migration re-read go back to inbound with the colleague as
-- counterparty, and the reminders it archived come back while their archive is
-- still the one it made. Both are found by the audit rows it wrote. Cleared
-- verdicts are not restored: the next owed, reply and settlement passes judge
-- those rows again.
SET LOCAL lock_timeout = '3s';

UPDATE activity r SET archived_at = NULL
  FROM audit_log archived
 WHERE archived.entity_id = r.id AND archived.actor_type = 'system' AND archived.action = 'archive'
   AND archived.after->>'migration' = '1791782100_a_colleagues_mail_is_ours_under_an_own_address'
   AND r.archived_at = archived.occurred_at;

WITH reread AS (
  SELECT a.id, a.sent_by_colleague AS colleague FROM activity a JOIN audit_log m ON m.entity_id = a.id
   WHERE m.actor_type = 'system' AND m.action = 'update'
     AND m.after->>'migration' = '1791782100_a_colleagues_mail_is_ours_under_an_own_address'
     AND a.sent_by_colleague IS NOT NULL
), unstamped AS (
  DELETE FROM activity_participant p USING reread
   WHERE p.activity_id = reread.id AND p.role = 'from' AND p.user_id = reread.colleague
)
UPDATE activity a
   SET direction = 'inbound',
       sent_by_colleague = NULL,
       counterparty_email = (SELECT lower(email) FROM app_user WHERE id = reread.colleague)
  FROM reread
 WHERE a.id = reread.id;
