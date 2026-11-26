-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '2s';

DROP TRIGGER IF EXISTS activity_link_project_last_activity_added ON activity_link;
DROP TRIGGER IF EXISTS activity_link_project_last_activity_moved ON activity_link;
DROP TRIGGER IF EXISTS activity_link_project_last_activity_removed ON activity_link;
DROP FUNCTION IF EXISTS trg_activity_link_project_last_activity_added();
DROP FUNCTION IF EXISTS trg_activity_link_project_last_activity_moved();
DROP FUNCTION IF EXISTS trg_activity_link_project_last_activity_removed();

CREATE OR REPLACE FUNCTION trg_activity_link_project_last_activity() RETURNS trigger
LANGUAGE plpgsql AS $$BEGIN
  IF TG_OP = 'INSERT' THEN
    IF NEW.project_id IS NOT NULL THEN
      PERFORM advance_last_activity('project'::regclass, NEW.project_id, last_activity_of_activity(NEW.activity_id));
    END IF;
    RETURN NULL;
  END IF;
  PERFORM move_project_last_activity(OLD.project_id);
  IF TG_OP = 'UPDATE' THEN
    PERFORM move_project_last_activity(NEW.project_id);
  END IF;
  RETURN NULL;
END;
$$;
CREATE TRIGGER activity_link_project_last_activity
	AFTER INSERT OR DELETE OR UPDATE ON activity_link
	FOR EACH ROW
	EXECUTE FUNCTION trg_activity_link_project_last_activity();

DROP TRIGGER IF EXISTS activity_link_last_activity_added ON activity_link;
DROP TRIGGER IF EXISTS activity_link_last_activity_moved ON activity_link;
DROP TRIGGER IF EXISTS activity_link_last_activity_removed ON activity_link;
DROP FUNCTION IF EXISTS trg_activity_link_last_activity_added();
DROP FUNCTION IF EXISTS trg_activity_link_last_activity_moved();
DROP FUNCTION IF EXISTS trg_activity_link_last_activity_removed();

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

CREATE OR REPLACE FUNCTION refresh_last_activity_for_link(cid uuid, did uuid, oid uuid) RETURNS void
LANGUAGE plpgsql AS $$
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
DROP FUNCTION IF EXISTS refresh_last_activity_for_links(uuid[], uuid[], uuid[]);

CREATE OR REPLACE FUNCTION advance_last_activity(tbl regclass, rid uuid, at timestamptz) RETURNS void
LANGUAGE plpgsql AS $$BEGIN
  IF rid IS NULL OR at IS NULL THEN RETURN; END IF;
  PERFORM set_config('margince.last_activity_move', 'on', true);
  CASE tbl
    WHEN 'contact'::regclass THEN
      PERFORM 1 FROM contact WHERE id = rid FOR UPDATE;
      UPDATE contact SET last_activity_at = at WHERE id = rid AND (last_activity_at IS NULL OR last_activity_at < at);
    WHEN 'deal'::regclass THEN
      PERFORM 1 FROM deal WHERE id = rid FOR UPDATE;
      UPDATE deal SET last_activity_at = at WHERE id = rid AND (last_activity_at IS NULL OR last_activity_at < at);
    WHEN 'company'::regclass THEN
      PERFORM 1 FROM company WHERE id = rid FOR UPDATE;
      UPDATE company SET last_activity_at = at WHERE id = rid AND (last_activity_at IS NULL OR last_activity_at < at);
    WHEN 'project'::regclass THEN
      PERFORM 1 FROM project WHERE id = rid FOR UPDATE;
      UPDATE project SET last_activity_at = at WHERE id = rid AND (last_activity_at IS NULL OR last_activity_at < at);
  END CASE;
  PERFORM set_config('margince.last_activity_move', 'off', true);
END;
$$;
DROP FUNCTION IF EXISTS advance_last_activities(regclass, uuid[], timestamptz[]);

CREATE OR REPLACE FUNCTION trg_activity_link_last_activity() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
  at timestamptz;
  deal_at timestamptz;
  reached record;
BEGIN
  IF TG_OP = 'INSERT' THEN
    IF NEW.contact_id IS NULL AND NEW.deal_id IS NULL AND NEW.company_id IS NULL THEN
      RETURN NULL;
    END IF;
    at := last_activity_of_activity(NEW.activity_id);
    IF NEW.deal_id IS NOT NULL THEN
      deal_at := last_activity_of_deal_activity(NEW.activity_id, NEW.deal_id);
    END IF;
    IF at IS NOT NULL OR deal_at IS NOT NULL THEN
      FOR reached IN
         SELECT reach.kind, reach.id
           FROM last_activity_reach(ARRAY[NEW.contact_id], ARRAY[NEW.deal_id], ARRAY[NEW.company_id]) reach
          ORDER BY reach.kind, reach.id
      LOOP
        PERFORM advance_last_activity(reached.kind::regclass, reached.id,
          CASE reached.kind WHEN 'company' THEN last_activity_of_company_activity(NEW.activity_id, reached.id)
                            WHEN 'deal' THEN deal_at ELSE at END);
      END LOOP;
    END IF;
    RETURN NULL;
  END IF;
  PERFORM refresh_last_activity_for_link(OLD.contact_id, OLD.deal_id, OLD.company_id);
  IF TG_OP = 'UPDATE' THEN
    PERFORM refresh_last_activity_for_link(NEW.contact_id, NEW.deal_id, NEW.company_id);
  END IF;
  RETURN NULL;
END;
$$;
CREATE TRIGGER activity_link_last_activity
	AFTER INSERT OR DELETE OR UPDATE ON activity_link
	FOR EACH ROW
	EXECUTE FUNCTION trg_activity_link_last_activity();
