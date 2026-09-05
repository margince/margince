-- A starter with no author cannot draft with anyone's authority. Pause old
-- instances until an authorized person explicitly enables and claims them.
SET LOCAL lock_timeout = '3s';

WITH paused AS (
    UPDATE automation
       SET enabled = false, version = version + 1, updated_at = now()
     WHERE enabled AND owner_id IS NULL AND archived_at IS NULL
       AND action ->> 'kind' = 'draft_email'
    RETURNING id
)
INSERT INTO audit_log (actor_type, actor_id, action, entity_type, entity_id, before, after)
SELECT 'system', 'migration', 'update', 'automation', id,
       '{"status":"enabled"}'::jsonb, '{"status":"paused"}'::jsonb
  FROM paused;
