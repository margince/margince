-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '2s';

-- Date writes change no parent key, so foreign-key key-share locks must pass.
-- Keep each installed calculation body while changing its row-lock mode.
DO $$
DECLARE
  clock_function regprocedure;
  definition text;
BEGIN
  FOREACH clock_function IN ARRAY ARRAY[
    'public.move_last_activity(regclass,uuid)'::regprocedure,
    'public.move_project_last_activity(uuid)'::regprocedure,
    'public.refresh_contact_strength_dates(uuid[])'::regprocedure,
    'public.touch_contact_strength_dates(uuid[],uuid[],timestamptz[])'::regprocedure
  ] LOOP
    definition := pg_get_functiondef(clock_function);
    IF strpos(definition, ' FOR UPDATE') = 0 OR strpos(definition, ' FOR NO KEY UPDATE') <> 0 THEN
      RAISE EXCEPTION 'unexpected row lock in %, check the function before migrating', clock_function;
    END IF;
    EXECUTE replace(definition, ' FOR UPDATE', ' FOR NO KEY UPDATE');
  END LOOP;
END;
$$;

-- The later strength trigger locks contacts even when their activity cannot
-- advance a clock, so those contacts must be locked before the project clock.
CREATE FUNCTION advance_last_activities(tbl regclass, rids uuid[], ats timestamptz[]) RETURNS void
LANGUAGE plpgsql AS $$
DECLARE
  moved_ids uuid[];
  moved_ats timestamptz[];
BEGIN
  SELECT array_agg(u.rid ORDER BY u.rid), array_agg(u.at ORDER BY u.rid)
    INTO moved_ids, moved_ats
    FROM (SELECT p.rid, max(p.at) AS at FROM unnest(rids, ats) p(rid, at)
           WHERE p.rid IS NOT NULL GROUP BY p.rid) u;
  IF moved_ids IS NULL THEN RETURN; END IF;
  PERFORM set_config('margince.last_activity_move', 'on', true);
  CASE tbl
    WHEN 'contact'::regclass THEN
      PERFORM 1 FROM contact WHERE id = ANY (moved_ids) ORDER BY id FOR NO KEY UPDATE;
      UPDATE contact SET last_activity_at = m.at FROM unnest(moved_ids, moved_ats) m(rid, at)
       WHERE contact.id = m.rid AND m.at IS NOT NULL
         AND (contact.last_activity_at IS NULL OR contact.last_activity_at < m.at);
    WHEN 'deal'::regclass THEN
      PERFORM 1 FROM deal WHERE id = ANY (moved_ids) ORDER BY id FOR NO KEY UPDATE;
      UPDATE deal SET last_activity_at = m.at FROM unnest(moved_ids, moved_ats) m(rid, at)
       WHERE deal.id = m.rid AND m.at IS NOT NULL
         AND (deal.last_activity_at IS NULL OR deal.last_activity_at < m.at);
    WHEN 'company'::regclass THEN
      PERFORM 1 FROM company WHERE id = ANY (moved_ids) ORDER BY id FOR NO KEY UPDATE;
      UPDATE company SET last_activity_at = m.at FROM unnest(moved_ids, moved_ats) m(rid, at)
       WHERE company.id = m.rid AND m.at IS NOT NULL
         AND (company.last_activity_at IS NULL OR company.last_activity_at < m.at);
    WHEN 'project'::regclass THEN
      PERFORM 1 FROM project WHERE id = ANY (moved_ids) ORDER BY id FOR NO KEY UPDATE;
      UPDATE project SET last_activity_at = m.at FROM unnest(moved_ids, moved_ats) m(rid, at)
       WHERE project.id = m.rid AND m.at IS NOT NULL
         AND (project.last_activity_at IS NULL OR project.last_activity_at < m.at);
  END CASE;
  PERFORM set_config('margince.last_activity_move', 'off', true);
END;
$$;

CREATE OR REPLACE FUNCTION advance_last_activity(tbl regclass, rid uuid, at timestamptz) RETURNS void
LANGUAGE plpgsql AS $$BEGIN
  PERFORM advance_last_activities(tbl, ARRAY[rid], ARRAY[at]);
END;
$$;

-- Strength dates can lock contacts whose clock inputs did not change.
-- Their locks share the clock's (kind, id) order before any project is reached.
CREATE FUNCTION refresh_last_activity_for_links(cids uuid[], dids uuid[], oids uuid[], strength_cids uuid[] DEFAULT NULL) RETURNS void
LANGUAGE plpgsql AS $$
DECLARE
  reached record;
BEGIN
  FOR reached IN
     SELECT r.kind, r.id, bool_or(r.move_clock) AS move_clock
       FROM (
         SELECT reach.kind, reach.id, true AS move_clock FROM last_activity_reach(cids, dids, oids) reach
         UNION ALL
         SELECT 'contact', cid, false FROM unnest(strength_cids) cid WHERE cid IS NOT NULL
       ) r
      GROUP BY r.kind, r.id ORDER BY r.kind, r.id
  LOOP
    IF reached.move_clock THEN
      PERFORM move_last_activity(reached.kind::regclass, reached.id);
    ELSE
      PERFORM 1 FROM contact WHERE id = reached.id FOR NO KEY UPDATE;
    END IF;
  END LOOP;
END;
$$;

CREATE OR REPLACE FUNCTION refresh_last_activity_for_link(cid uuid, did uuid, oid uuid) RETURNS void
LANGUAGE plpgsql AS $$BEGIN
  PERFORM refresh_last_activity_for_links(ARRAY[cid], ARRAY[did], ARRAY[oid]);
END;
$$;

CREATE OR REPLACE FUNCTION trg_activity_last_activity() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
  cids uuid[];
  dids uuid[];
  oids uuid[];
  pids uuid[];
  pid uuid;
BEGIN
  -- Transition tables require the changed-field guard inside the trigger function.
  WITH changed AS (
    SELECT n.id,
           ROW(o.occurred_at, o.archived_at, o.audience, o.origin, o.kind, o.meeting_status,
               o.occurred_precision, o.call_outcome, o.bulk_mail_attested)
           IS DISTINCT FROM
           ROW(n.occurred_at, n.archived_at, n.audience, n.origin, n.kind, n.meeting_status,
               n.occurred_precision, n.call_outcome, n.bulk_mail_attested) AS clock_changed,
           ROW(o.audience_reason, o.thread_key)
           IS DISTINCT FROM ROW(n.audience_reason, n.thread_key) AS scope_changed
      FROM clock_before o JOIN clock_after n ON n.id = o.id
  )
  SELECT array_agg(l.contact_id) FILTER (WHERE l.contact_id IS NOT NULL),
         array_agg(l.deal_id) FILTER (WHERE l.deal_id IS NOT NULL),
         array_agg(l.company_id) FILTER (WHERE l.company_id IS NOT NULL),
         array_agg(l.project_id) FILTER (WHERE c.clock_changed AND l.project_id IS NOT NULL)
    INTO cids, dids, oids, pids
    FROM changed c JOIN activity_link l ON l.activity_id = c.id
   WHERE c.clock_changed OR c.scope_changed;
  IF cids IS NULL AND dids IS NULL AND oids IS NULL AND pids IS NULL THEN RETURN NULL; END IF;
  PERFORM refresh_last_activity_for_links(cids, dids, oids,
    ARRAY(SELECT l.contact_id FROM clock_after n JOIN activity_link l ON l.activity_id = n.id
           WHERE l.contact_id IS NOT NULL));
  FOR pid IN SELECT DISTINCT p.id FROM unnest(pids) p(id) WHERE p.id IS NOT NULL ORDER BY p.id LOOP
    PERFORM move_project_last_activity(pid);
  END LOOP;
  RETURN NULL;
END;
$$;

DROP TRIGGER activity_last_activity ON activity;
DROP TRIGGER activity_project_last_activity ON activity;
DROP FUNCTION trg_activity_project_last_activity();
CREATE TRIGGER activity_last_activity
  AFTER UPDATE ON activity REFERENCING OLD TABLE AS clock_before NEW TABLE AS clock_after
  FOR EACH STATEMENT EXECUTE FUNCTION trg_activity_last_activity();

CREATE FUNCTION trg_activity_link_last_activity_added() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
  moved record;
BEGIN
  FOR moved IN
     WITH timed AS (
       SELECT n.activity_id, n.contact_id, n.deal_id, n.company_id,
              last_activity_of_activity(n.activity_id) AS at,
              CASE WHEN n.deal_id IS NOT NULL THEN last_activity_of_deal_activity(n.activity_id, n.deal_id) END AS deal_at
         FROM added n
        WHERE n.contact_id IS NOT NULL OR n.deal_id IS NOT NULL OR n.company_id IS NOT NULL
     ), reached AS (
       SELECT reach.kind, reach.id,
              CASE reach.kind WHEN 'company' THEN last_activity_of_company_activity(t.activity_id, reach.id)
                              WHEN 'deal' THEN t.deal_at ELSE t.at END AS at
         FROM timed t
        CROSS JOIN LATERAL last_activity_reach(ARRAY[t.contact_id], ARRAY[t.deal_id], ARRAY[t.company_id]) reach
     )
     SELECT reached.kind, array_agg(reached.id) AS ids, array_agg(reached.at) AS ats
       FROM reached GROUP BY reached.kind ORDER BY reached.kind
  LOOP
    PERFORM advance_last_activities(moved.kind::regclass, moved.ids, moved.ats);
  END LOOP;
  RETURN NULL;
END;
$$;

CREATE FUNCTION trg_activity_link_last_activity_moved() RETURNS trigger
LANGUAGE plpgsql AS $$BEGIN
  PERFORM refresh_last_activity_for_links(
    ARRAY(SELECT contact_id FROM moved_from UNION SELECT contact_id FROM moved_to),
    ARRAY(SELECT deal_id FROM moved_from UNION SELECT deal_id FROM moved_to),
    ARRAY(SELECT company_id FROM moved_from UNION SELECT company_id FROM moved_to));
  RETURN NULL;
END;
$$;

CREATE FUNCTION trg_activity_link_last_activity_removed() RETURNS trigger
LANGUAGE plpgsql AS $$BEGIN
  PERFORM refresh_last_activity_for_links(
    ARRAY(SELECT DISTINCT contact_id FROM removed),
    ARRAY(SELECT DISTINCT deal_id FROM removed),
    ARRAY(SELECT DISTINCT company_id FROM removed));
  RETURN NULL;
END;
$$;

DROP TRIGGER activity_link_last_activity ON activity_link;
DROP FUNCTION trg_activity_link_last_activity();

CREATE TRIGGER activity_link_last_activity_added
	AFTER INSERT ON activity_link REFERENCING NEW TABLE AS added
	FOR EACH STATEMENT EXECUTE FUNCTION trg_activity_link_last_activity_added();
CREATE TRIGGER activity_link_last_activity_moved
	AFTER UPDATE ON activity_link REFERENCING OLD TABLE AS moved_from NEW TABLE AS moved_to
	FOR EACH STATEMENT EXECUTE FUNCTION trg_activity_link_last_activity_moved();
CREATE TRIGGER activity_link_last_activity_removed
	AFTER DELETE ON activity_link REFERENCING OLD TABLE AS removed
	FOR EACH STATEMENT EXECUTE FUNCTION trg_activity_link_last_activity_removed();

CREATE FUNCTION trg_activity_link_project_last_activity_added() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
  project_ids uuid[];
  project_ats timestamptz[];
BEGIN
  SELECT array_agg(n.project_id), array_agg(last_activity_of_activity(n.activity_id))
    INTO project_ids, project_ats
    FROM added n WHERE n.project_id IS NOT NULL;
  PERFORM advance_last_activities('project'::regclass, project_ids, project_ats);
  RETURN NULL;
END;
$$;

CREATE FUNCTION trg_activity_link_project_last_activity_moved() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
  pid uuid;
BEGIN
  FOR pid IN SELECT project_id FROM moved_from UNION SELECT project_id FROM moved_to ORDER BY 1 LOOP
    PERFORM move_project_last_activity(pid);
  END LOOP;
  RETURN NULL;
END;
$$;

CREATE FUNCTION trg_activity_link_project_last_activity_removed() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
  pid uuid;
BEGIN
  FOR pid IN SELECT DISTINCT project_id FROM removed ORDER BY 1 LOOP
    PERFORM move_project_last_activity(pid);
  END LOOP;
  RETURN NULL;
END;
$$;

DROP TRIGGER activity_link_project_last_activity ON activity_link;
DROP FUNCTION trg_activity_link_project_last_activity();

CREATE TRIGGER activity_link_project_last_activity_added
	AFTER INSERT ON activity_link REFERENCING NEW TABLE AS added
	FOR EACH STATEMENT EXECUTE FUNCTION trg_activity_link_project_last_activity_added();
CREATE TRIGGER activity_link_project_last_activity_moved
	AFTER UPDATE ON activity_link REFERENCING OLD TABLE AS moved_from NEW TABLE AS moved_to
	FOR EACH STATEMENT EXECUTE FUNCTION trg_activity_link_project_last_activity_moved();
CREATE TRIGGER activity_link_project_last_activity_removed
	AFTER DELETE ON activity_link REFERENCING OLD TABLE AS removed
	FOR EACH STATEMENT EXECUTE FUNCTION trg_activity_link_project_last_activity_removed();
