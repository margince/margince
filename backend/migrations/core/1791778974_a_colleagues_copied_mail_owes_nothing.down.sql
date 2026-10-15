-- The reminders this archived come back, found by the audit rows it wrote, and
-- only while their archive is still the one it made: the same transaction time
-- stamped both. A reminder archived again since stays archived. The
-- deleted settlement verdicts are not restored: the next settlement pass judges
-- those requests again.
SET LOCAL lock_timeout = '3s';

UPDATE activity r SET archived_at = NULL
  FROM audit_log archived
 WHERE archived.entity_id = r.id AND archived.actor_type = 'system' AND archived.action = 'archive'
   AND archived.after->>'migration' = '1791778974_a_colleagues_copied_mail_owes_nothing'
   AND r.archived_at = archived.occurred_at;
