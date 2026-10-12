SET LOCAL lock_timeout = '5s';

-- The refresh restored as 1790175606 left it, with its reach inline, and the
-- two functions this migration added dropped once nothing calls them.
CREATE OR REPLACE FUNCTION refresh_last_activity_for_link(cid uuid, did uuid, oid uuid) RETURNS void
    LANGUAGE plpgsql
    AS $$
DECLARE
  reached record;
BEGIN
  FOR reached IN
     SELECT kind, id FROM (
       SELECT 'contact' AS kind, cid AS id WHERE cid IS NOT NULL
       UNION SELECT 'deal', did WHERE did IS NOT NULL
       UNION SELECT 'company', oid WHERE oid IS NOT NULL
       UNION SELECT 'company', d.company_id FROM deal d
              WHERE d.id = did AND d.company_id IS NOT NULL
       UNION SELECT 'company', r.company_id FROM relationship r
              WHERE r.contact_id = cid AND r.kind = 'employment'
                AND r.ended_at IS NULL AND r.archived_at IS NULL
     ) reach ORDER BY kind, id
  LOOP
    PERFORM move_last_activity(reached.kind::regclass, reached.id);
  END LOOP;
END;
$$;

DROP FUNCTION lock_last_activity_targets(uuid[], uuid[], uuid[], uuid[]);
DROP FUNCTION last_activity_reach(uuid[], uuid[], uuid[]);
