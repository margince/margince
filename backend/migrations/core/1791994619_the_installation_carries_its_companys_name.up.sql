-- The installation's name is the anchor company's display name. A human saving
-- the company profile writes both; this carries a name already saved onto the
-- setting. With no anchor the configured name stands, and a name the setting's
-- own rule refuses (blank, past 200 characters) is left for the next save.
--
-- Identity refuses to boot beside a second live workspace, so the one live
-- workspace is the installation's, and the audit row names it as SetRawTx does.
WITH installation AS (
  SELECT id FROM workspace WHERE archived_at IS NULL ORDER BY created_at LIMIT 1
), anchor AS (
  SELECT to_jsonb(display_name) AS name
    FROM company
   WHERE is_anchor AND archived_at IS NULL
     AND btrim(display_name) <> ''
     AND char_length(btrim(display_name)) <= 200
     AND EXISTS (SELECT 1 FROM installation)
), previous AS (
  SELECT value FROM setting WHERE key = 'installation.name'
), written AS (
  INSERT INTO setting (key, value)
  SELECT 'installation.name', anchor.name FROM anchor
  ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now()
   WHERE setting.value IS DISTINCT FROM EXCLUDED.value
  RETURNING value
)
INSERT INTO audit_log (actor_type, actor_id, action, entity_type, entity_id, before, after)
SELECT 'system', 'migration', 'update', 'installation_settings', installation.id,
       jsonb_build_object('installation.name', coalesce((SELECT value FROM previous), '""'::jsonb)),
       jsonb_build_object('installation.name', written.value)
  FROM written
 CROSS JOIN installation;
