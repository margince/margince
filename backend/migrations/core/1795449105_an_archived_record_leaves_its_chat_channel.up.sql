SET LOCAL lock_timeout = '3s';
-- An archived record leaves its link; a link left with no live record is retired,
-- which frees its channel. Audited as the migration's, with the store's image.
WITH retired AS (
  UPDATE chat_link l
     SET archived_at = now()
   WHERE l.archived_at IS NULL
     AND NOT EXISTS (SELECT 1 FROM deal d WHERE d.id = l.deal_id AND d.archived_at IS NULL)
     AND NOT EXISTS (SELECT 1 FROM project p WHERE p.id = l.project_id AND p.archived_at IS NULL)
  RETURNING l.id, l.provider, l.channel_slug, l.channel_name, l.deal_id, l.project_id, l.verbosity, l.status
)
INSERT INTO audit_log (actor_type, actor_id, action, entity_type, entity_id, before, after)
SELECT 'system', 'migration', 'archive', 'chat_link', id,
       jsonb_build_object('provider', provider, 'channel_slug', channel_slug, 'channel_name', channel_name,
                          'deal_id', deal_id, 'project_id', project_id, 'verbosity', verbosity, 'status', status),
       NULL
  FROM retired;

WITH left_deal AS (
  UPDATE chat_link l
     SET deal_id = NULL
    FROM deal d
   WHERE d.id = l.deal_id AND d.archived_at IS NOT NULL AND l.archived_at IS NULL
  RETURNING l.id, l.provider, l.channel_slug, l.channel_name, d.id AS deal_id, l.project_id, l.verbosity, l.status
)
INSERT INTO audit_log (actor_type, actor_id, action, entity_type, entity_id, before, after)
SELECT 'system', 'migration', 'update', 'chat_link', id,
       jsonb_build_object('provider', provider, 'channel_slug', channel_slug, 'channel_name', channel_name,
                          'deal_id', deal_id, 'project_id', project_id, 'verbosity', verbosity, 'status', status),
       jsonb_build_object('provider', provider, 'channel_slug', channel_slug, 'channel_name', channel_name,
                          'deal_id', NULL, 'project_id', project_id, 'verbosity', verbosity, 'status', status)
  FROM left_deal;

WITH left_project AS (
  UPDATE chat_link l
     SET project_id = NULL
    FROM project p
   WHERE p.id = l.project_id AND p.archived_at IS NOT NULL AND l.archived_at IS NULL
  RETURNING l.id, l.provider, l.channel_slug, l.channel_name, l.deal_id, p.id AS project_id, l.verbosity, l.status
)
INSERT INTO audit_log (actor_type, actor_id, action, entity_type, entity_id, before, after)
SELECT 'system', 'migration', 'update', 'chat_link', id,
       jsonb_build_object('provider', provider, 'channel_slug', channel_slug, 'channel_name', channel_name,
                          'deal_id', deal_id, 'project_id', project_id, 'verbosity', verbosity, 'status', status),
       jsonb_build_object('provider', provider, 'channel_slug', channel_slug, 'channel_name', channel_name,
                          'deal_id', deal_id, 'project_id', NULL, 'verbosity', verbosity, 'status', status)
  FROM left_project;
