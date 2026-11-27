-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '2s';

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
      PERFORM 1 FROM contact WHERE id = ANY (moved_ids) ORDER BY id FOR UPDATE;
      UPDATE contact SET last_activity_at = m.at FROM unnest(moved_ids, moved_ats) m(rid, at)
       WHERE contact.id = m.rid AND m.at IS NOT NULL
         AND (contact.last_activity_at IS NULL OR contact.last_activity_at < m.at);
    WHEN 'deal'::regclass THEN
      PERFORM 1 FROM deal WHERE id = ANY (moved_ids) ORDER BY id FOR UPDATE;
      UPDATE deal SET last_activity_at = m.at FROM unnest(moved_ids, moved_ats) m(rid, at)
       WHERE deal.id = m.rid AND m.at IS NOT NULL
         AND (deal.last_activity_at IS NULL OR deal.last_activity_at < m.at);
    WHEN 'company'::regclass THEN
      PERFORM 1 FROM company WHERE id = ANY (moved_ids) ORDER BY id FOR UPDATE;
      UPDATE company SET last_activity_at = m.at FROM unnest(moved_ids, moved_ats) m(rid, at)
       WHERE company.id = m.rid AND m.at IS NOT NULL
         AND (company.last_activity_at IS NULL OR company.last_activity_at < m.at);
    WHEN 'project'::regclass THEN
      PERFORM 1 FROM project WHERE id = ANY (moved_ids) ORDER BY id FOR UPDATE;
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

-- Recomputes every clock the named records reach, each once, in (kind, id) order.
CREATE FUNCTION refresh_last_activity_for_links(cids uuid[], dids uuid[], oids uuid[]) RETURNS void
LANGUAGE plpgsql AS $$
DECLARE
  reached record;
BEGIN
  FOR reached IN
     SELECT reach.kind, reach.id
       FROM last_activity_reach(cids, dids, oids) reach
      ORDER BY reach.kind, reach.id
  LOOP
    PERFORM move_last_activity(reached.kind::regclass, reached.id);
  END LOOP;
END;
$$;

CREATE OR REPLACE FUNCTION refresh_last_activity_for_link(cid uuid, did uuid, oid uuid) RETURNS void
LANGUAGE plpgsql AS $$BEGIN
  PERFORM refresh_last_activity_for_links(ARRAY[cid], ARRAY[did], ARRAY[oid]);
END;
$$;

CREATE OR REPLACE FUNCTION trg_activity_last_activity() RETURNS trigger
LANGUAGE plpgsql AS $$BEGIN
  PERFORM refresh_last_activity_for_links(
    ARRAY(SELECT contact_id FROM activity_link WHERE activity_id = NEW.id AND contact_id IS NOT NULL),
    ARRAY(SELECT deal_id FROM activity_link WHERE activity_id = NEW.id AND deal_id IS NOT NULL),
    ARRAY(SELECT company_id FROM activity_link WHERE activity_id = NEW.id AND company_id IS NOT NULL));
  RETURN NULL;
END;
$$;

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
