SET LOCAL lock_timeout = '3s';

-- Three facts that only the workspace table still holds move into
-- installation_state, so the table can go later: the id that stored object keys
-- and sealed secrets are scoped by, when the installation's trial began, and
-- how many archived companies it carries.
--
-- The id is the live row's, the same row identity.InstallationWorkspace
-- chooses. An installation with two live rows cannot run today, so the
-- migration refuses it rather than guess which company's stored keys to keep.
-- An empty installation writes nothing here; the first setup writes the keys.
DO $$
BEGIN
    IF (SELECT count(*) FROM workspace WHERE archived_at IS NULL) > 1 THEN
        RAISE EXCEPTION 'more than one live workspace: archive all but one before upgrading';
    END IF;
END
$$;

INSERT INTO installation_state (key, value)
SELECT 'stored_key_workspace', jsonb_build_object('id', id)
  FROM workspace
 WHERE archived_at IS NULL
ON CONFLICT (key) DO NOTHING;

INSERT INTO installation_state (key, value)
SELECT 'installation_trial', jsonb_build_object('started_at', min(created_at))
  FROM workspace
HAVING count(*) > 0
ON CONFLICT (key) DO NOTHING;

INSERT INTO installation_state (key, value)
SELECT 'archived_predecessor', jsonb_build_object('archived_companies', count(*))
  FROM workspace
 WHERE archived_at IS NOT NULL
HAVING count(*) > 0
ON CONFLICT (key) DO NOTHING;
