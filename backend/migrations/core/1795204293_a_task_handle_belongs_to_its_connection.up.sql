-- A task handle and a retry key belong to the connected agent, not to one
-- passport row. A token refresh retires the passport and mints another under the
-- same OAuth grant, so the grant is what lets the agent find both afterwards.
SET LOCAL lock_timeout = '3s';

ALTER TABLE agent_task
    ADD COLUMN connection_id uuid
        CONSTRAINT agent_task_connection_fk REFERENCES oauth_grant(id) ON DELETE RESTRICT;

-- Live tasks of a connected agent take its connection, so a handle handed out
-- before this change still answers after the next refresh.
UPDATE agent_task t
   SET connection_id = p.oauth_grant_id
  FROM passport p
 WHERE p.id = t.passport_id
   AND p.oauth_grant_id IS NOT NULL
   AND t.expires_at > now();

-- A retry key a connected agent spent inside the 24-hour replay window moves to
-- its connection, the holder both doors key on now. Where two of its passports
-- spent one key on one path, the newest claim moves and the older one stays.
WITH moving AS (
    SELECT DISTINCT ON (p.oauth_grant_id, k.key, k.endpoint)
           k.principal_id, k.key, k.endpoint, 'agent:' || p.oauth_grant_id::text AS holder
      FROM idempotency_key k
      JOIN passport p ON k.principal_id = 'agent:' || p.id::text
     WHERE p.oauth_grant_id IS NOT NULL
       AND k.created_at > now() - interval '24 hours'
     ORDER BY p.oauth_grant_id, k.key, k.endpoint, k.created_at DESC
)
UPDATE idempotency_key k
   SET principal_id = m.holder
  FROM moving m
 WHERE k.principal_id = m.principal_id
   AND k.key = m.key
   AND k.endpoint = m.endpoint
   AND NOT EXISTS (
        SELECT 1 FROM idempotency_key held
         WHERE held.principal_id = m.holder AND held.key = m.key AND held.endpoint = m.endpoint);
