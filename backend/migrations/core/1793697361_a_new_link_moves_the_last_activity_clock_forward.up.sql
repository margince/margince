-- A new activity link can only move a record's last-activity clock forward, so
-- an insert compares the one activity's time with the stored clock. Each insert
-- used to fold the record's whole history, which made a bulk import for a busy
-- contact cost the square of that history. Delete and update still refold,
-- because they can move the clock back. An activity whose time comes later is
-- refolded then (contacts.RefoldClocksTx), as before.

-- The time one linked activity moves the clock to, by the rule the other
-- last_activity_of_* functions fold with, or NULL where it does not count. It
-- reads through the link, in their shape, so the gates hold its arm to theirs.
CREATE OR REPLACE FUNCTION last_activity_of_activity(aid uuid) RETURNS timestamptz
LANGUAGE sql STABLE AS $$
  SELECT max(a.occurred_at)
    FROM activity_link l
    JOIN activity a ON a.id = l.activity_id AND a.archived_at IS NULL
     AND a.audience = 'workspace'
     AND a.origin NOT IN ('system_remediation', 'system_notice')
     AND (a.kind IN ('email','message','meeting','call','note') AND (a.kind <> 'meeting' OR (a.meeting_status IS NULL OR a.meeting_status IN ('booked','held'))) AND ((a.kind = 'meeting' AND (a.occurred_at <= greatest(now(), ((date_trunc('day', CAST(now() AS timestamptz) AT TIME ZONE COALESCE((SELECT st.value #>> '{}' FROM setting st WHERE st.key = 'installation.timezone'), 'UTC')) + interval '12 hours') AT TIME ZONE 'UTC')) AND (a.occurred_precision = 'day' AND a.occurred_at < ((date_trunc('day', CAST(now() AS timestamptz) AT TIME ZONE COALESCE((SELECT st.value #>> '{}' FROM setting st WHERE st.key = 'installation.timezone'), 'UTC')) + interval '12 hours') AT TIME ZONE 'UTC') OR a.occurred_precision = 'instant' AND a.occurred_at <= now()))) OR (a.kind <> 'meeting' AND (a.occurred_at <= greatest(now(), ((date_trunc('day', CAST(now() AS timestamptz) AT TIME ZONE COALESCE((SELECT st.value #>> '{}' FROM setting st WHERE st.key = 'installation.timezone'), 'UTC')) + interval '12 hours') AT TIME ZONE 'UTC')) AND (a.occurred_precision = 'day' OR a.occurred_at <= now())))))
   WHERE l.activity_id = aid
$$;

-- advance_last_activity moves one record's clock to at when that is later. It
-- locks the row first, as move_last_activity does, in the caller's order.
CREATE OR REPLACE FUNCTION advance_last_activity(tbl regclass, rid uuid, at timestamptz) RETURNS void
LANGUAGE plpgsql AS $$
BEGIN
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

CREATE OR REPLACE FUNCTION trg_activity_link_last_activity() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
  at timestamptz;
  reached record;
BEGIN
  IF TG_OP = 'INSERT' THEN
    at := last_activity_of_activity(NEW.activity_id);
    IF at IS NOT NULL THEN
      FOR reached IN
         SELECT reach.kind, reach.id
           FROM last_activity_reach(ARRAY[NEW.contact_id], ARRAY[NEW.deal_id], ARRAY[NEW.company_id]) reach
          ORDER BY reach.kind, reach.id
      LOOP
        PERFORM advance_last_activity(reached.kind::regclass, reached.id, at);
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

CREATE OR REPLACE FUNCTION trg_activity_link_project_last_activity() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  IF TG_OP = 'INSERT' THEN
    PERFORM advance_last_activity('project'::regclass, NEW.project_id, last_activity_of_activity(NEW.activity_id));
    RETURN NULL;
  END IF;
  PERFORM move_project_last_activity(OLD.project_id);
  IF TG_OP = 'UPDATE' THEN
    PERFORM move_project_last_activity(NEW.project_id);
  END IF;
  RETURN NULL;
END;
$$;
