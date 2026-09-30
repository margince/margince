SET LOCAL lock_timeout = '5s';

-- The last-activity refresh takes its row locks in ONE global order.
--
-- Every record it touches is locked FOR UPDATE, and until now the sequence was
-- positional: contact, then deal, then the companies. Within one firing the
-- company loop ordered itself by id and said why — "two writers reaching the
-- same accounts lock them in the same order, so they queue rather than
-- deadlock" — and that property held for the companies alone.
--
-- Across the three TYPES it did not hold at all, because the sequence was the
-- order the arms are written rather than an order both writers agree on. Two
-- transactions reaching the same records through different link rows lock them
-- in different sequences and Postgres aborts one of them:
--
--   A logs an activity linked to [deal D, contact X] — locks D, waits on X.
--   B logs an activity linked to [contact X, deal D] — locks X, waits on D.
--
-- So the reached set is collected first and locked in (type, id) order. The
-- type names are the ones move_last_activity already switches on, and the
-- ordering is over the pair rather than the id alone: two ids are comparable
-- across tables, and a contact whose uuid sorts below a deal's must still be
-- locked in the same relative position by every writer.
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

-- The activity trigger's own scan, ordered for the same reason.
--
-- It calls the refresh once per link row of one activity, and the rows came
-- back in whatever order the scan produced. Two transactions re-dating or
-- archiving activities that share several targets therefore reached those
-- targets in different sequences — the same cycle as above, arriving through
-- the other trigger.
--
-- Ordered by the link's own columns, which is the order the refresh's first
-- lock follows from: a link carries exactly one of the three, so sorting the
-- rows sorts the first record each firing will take.
CREATE OR REPLACE FUNCTION trg_activity_last_activity() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
  l record;
BEGIN
  FOR l IN
     SELECT contact_id, deal_id, company_id FROM activity_link
      WHERE activity_id = NEW.id
      ORDER BY contact_id, deal_id, company_id
  LOOP
    PERFORM refresh_last_activity_for_link(l.contact_id, l.deal_id, l.company_id);
  END LOOP;
  RETURN NULL;
END;
$$;
