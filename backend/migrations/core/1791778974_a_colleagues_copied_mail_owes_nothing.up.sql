-- A colleague's copied mail that migration 1791775182 re-read as outbound is no
-- longer a request, so what the owed pass left on it while it read as one goes:
-- its settlement verdicts, and the reminders the system filed for it that no
-- human touched. A reminder a human wrote or edited stays theirs.
--
-- Idempotent: a second run finds nothing to delete and no open reminder.
SET LOCAL lock_timeout = '3s';

DELETE FROM activity_request_settlement s USING activity a
 WHERE s.request_activity_id = a.id AND a.sent_by_colleague IS NOT NULL;

WITH retired AS (
  UPDATE activity r
     SET archived_at = now()
    FROM activity a
   WHERE a.sent_by_colleague IS NOT NULL
     AND r.source_system = 'email_request' AND r.source_activity_id = a.id
     AND NOT r.is_done AND r.archived_at IS NULL
     AND NOT EXISTS (SELECT 1 FROM audit_log worked
           WHERE worked.entity_type = 'activity' AND worked.entity_id = r.id AND worked.actor_type = 'human')
  RETURNING r.id
)
INSERT INTO audit_log (actor_type, actor_id, action, entity_type, entity_id, after)
SELECT 'system', 'migration', 'archive', 'activity', id,
       jsonb_build_object('migration', '1791778974_a_colleagues_copied_mail_owes_nothing')
  FROM retired;
