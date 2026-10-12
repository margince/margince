-- The reminders this archived come back, found by the audit rows it wrote. The
-- deleted settlement verdicts are not restored: the next settlement pass judges
-- those requests again.
SET LOCAL lock_timeout = '3s';

UPDATE activity r SET archived_at = NULL
  FROM audit_log archived
 WHERE archived.entity_id = r.id AND archived.actor_type = 'system' AND archived.action = 'archive'
   AND archived.after->>'migration' = '1791778974_a_colleagues_copied_mail_owes_nothing';
