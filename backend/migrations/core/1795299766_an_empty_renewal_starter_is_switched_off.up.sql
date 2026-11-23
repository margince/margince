SET LOCAL lock_timeout = '3s';
-- A renewal starter seeded switched on, with no record type and no date field,
-- scans nothing, so switching it off stops no working rule. A rule that names
-- both, or any other rule, keeps its flag. The write is audited as the migration's.
WITH paused AS (
  UPDATE automation
     SET enabled = false, version = version + 1, updated_at = now()
   WHERE key = 'renewal_reminder'
     AND origin = 'catalog'
     AND enabled
     AND archived_at IS NULL
     AND (COALESCE(btrim(params ->> 'object'), '') = '' OR COALESCE(btrim(params ->> 'date_field'), '') = '')
  RETURNING id
)
INSERT INTO audit_log (actor_type, actor_id, action, entity_type, entity_id, before, after)
SELECT 'system', 'migration', 'pause', 'automation', paused.id,
       jsonb_build_object('enabled', true), jsonb_build_object('enabled', false)
  FROM paused;
