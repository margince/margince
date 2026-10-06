SET LOCAL lock_timeout = '5s';

-- A writer locks EVERY record its last-activity refresh will reach, once, in
-- one (kind, id) order, before it writes anything that fires the refresh
-- (#6282).
--
-- 1790175606 ordered the records ONE link reaches, and the writer orders the
-- links. Neither orders the whole activity, because each link's firing locks
-- what it reaches on its own: a contact link also locks the contact's current
-- employers, a deal link the deal's company. So
--
--   A logs [company C2, contact X @ C1] — locks C2, then (through X) C1;
--   B logs [company C1, contact Y @ C2] — locks C1, then (through Y) C2;
--
-- and each sorted its own links correctly while the pair still deadlocks.
--
-- The second cycle needs no second record at all. Every attach probe takes FOR
-- SHARE on its target (auth.EnsureAttachTarget, EnsureLinkTarget) BEFORE the
-- trigger takes FOR UPDATE on it, so two writers naming one company each hold
-- a share the other's upgrade must wait out. That is the relationship path's
-- deadlock too: two employments at one company share it in the endpoint
-- probe and upgrade in trg_relationship_last_activity. A hot account under a
-- parallel import is exactly where both land.
--
-- lock_last_activity_targets closes both: taken first, it holds FOR UPDATE on
-- the whole reached set, so the probes' shares and the triggers' own locks are
-- re-locks of rows the transaction already owns, and the only locks acquired
-- while holding others are acquired in the one order every writer agrees on.
--
-- The reach is spelled ONCE, in last_activity_reach, and the refresh reads it
-- too. A lock set computed from a second copy would drift from what the
-- trigger actually touches, and every record it missed would be a lock taken
-- out of order again.
--
-- What it cannot close: an employment committed between the reach being read
-- and the refresh firing adds an employer the lock never saw. The contact is
-- locked FOR UPDATE by then and an employment writer locks it too, so the
-- window is one writer that got there first; the API retries the transaction
-- on 40P01 for that remainder.
CREATE FUNCTION last_activity_reach(contact_ids uuid[], deal_ids uuid[], company_ids uuid[])
    RETURNS TABLE (kind text, id uuid)
    LANGUAGE sql STABLE
    AS $$
  SELECT 'contact', named FROM unnest(contact_ids) named WHERE named IS NOT NULL
  UNION SELECT 'deal', named FROM unnest(deal_ids) named WHERE named IS NOT NULL
  UNION SELECT 'company', named FROM unnest(company_ids) named WHERE named IS NOT NULL
  UNION SELECT 'company', d.company_id FROM deal d
         WHERE d.id = ANY (deal_ids) AND d.company_id IS NOT NULL
  UNION SELECT 'company', r.company_id FROM relationship r
         WHERE r.contact_id = ANY (contact_ids) AND r.kind = 'employment'
           AND r.ended_at IS NULL AND r.archived_at IS NULL
$$;

-- The same FOR UPDATE move_last_activity and move_project_last_activity take,
-- over the union of everything the writer's links reach. A project reaches
-- nothing further, but its link is probed FOR SHARE and upgraded by its own
-- trigger exactly as a company's is, so it is locked here with the rest.
CREATE FUNCTION lock_last_activity_targets(contact_ids uuid[], deal_ids uuid[], company_ids uuid[], project_ids uuid[])
    RETURNS void
    LANGUAGE plpgsql
    AS $$
DECLARE
  reached record;
BEGIN
  FOR reached IN
     SELECT reach.kind, reach.id FROM last_activity_reach(contact_ids, deal_ids, company_ids) reach
     UNION SELECT 'project', named FROM unnest(project_ids) named WHERE named IS NOT NULL
     ORDER BY 1, 2
  LOOP
    CASE reached.kind
      WHEN 'company' THEN PERFORM 1 FROM company WHERE id = reached.id FOR UPDATE;
      WHEN 'contact' THEN PERFORM 1 FROM contact WHERE id = reached.id FOR UPDATE;
      WHEN 'deal' THEN PERFORM 1 FROM deal WHERE id = reached.id FOR UPDATE;
      WHEN 'project' THEN PERFORM 1 FROM project WHERE id = reached.id FOR UPDATE;
    END CASE;
  END LOOP;
END;
$$;

-- Unchanged in behaviour: the same arms, in the same (kind, id) order, now
-- read from the one spelling of the reach.
CREATE OR REPLACE FUNCTION refresh_last_activity_for_link(cid uuid, did uuid, oid uuid) RETURNS void
    LANGUAGE plpgsql
    AS $$
DECLARE
  reached record;
BEGIN
  FOR reached IN
     SELECT reach.kind, reach.id
       FROM last_activity_reach(ARRAY[cid], ARRAY[did], ARRAY[oid]) reach
      ORDER BY reach.kind, reach.id
  LOOP
    PERFORM move_last_activity(reached.kind::regclass, reached.id);
  END LOOP;
END;
$$;
