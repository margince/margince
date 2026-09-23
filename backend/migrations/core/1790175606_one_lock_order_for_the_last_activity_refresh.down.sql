SET LOCAL lock_timeout = '5s';

-- Restored as they stood: positional arms, and an unordered scan.
CREATE OR REPLACE FUNCTION refresh_last_activity_for_link(cid uuid, did uuid, oid uuid) RETURNS void
    LANGUAGE plpgsql
    AS $$
DECLARE
  reached uuid;
BEGIN
  PERFORM move_last_activity('contact', cid);
  PERFORM move_last_activity('deal', did);
  FOR reached IN
     SELECT x FROM (
       SELECT oid AS x WHERE oid IS NOT NULL
       UNION SELECT d.company_id FROM deal d WHERE d.id = did AND d.company_id IS NOT NULL
       UNION SELECT r.company_id FROM relationship r
              WHERE r.contact_id = cid AND r.kind = 'employment' AND r.ended_at IS NULL AND r.archived_at IS NULL
     ) reach ORDER BY x
  LOOP
    PERFORM move_last_activity('company', reached);
  END LOOP;
END;
$$;

CREATE OR REPLACE FUNCTION trg_activity_last_activity() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
  PERFORM refresh_last_activity_for_link(l.contact_id, l.deal_id, l.company_id)
     FROM activity_link l WHERE l.activity_id = NEW.id;
  RETURN NULL;
END;
$$;
