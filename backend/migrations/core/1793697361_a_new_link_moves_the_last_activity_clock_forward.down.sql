CREATE OR REPLACE FUNCTION trg_activity_link_last_activity() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  IF TG_OP IN ('DELETE', 'UPDATE') THEN
    PERFORM refresh_last_activity_for_link(OLD.contact_id, OLD.deal_id, OLD.company_id);
  END IF;
  IF TG_OP IN ('INSERT', 'UPDATE') THEN
    PERFORM refresh_last_activity_for_link(NEW.contact_id, NEW.deal_id, NEW.company_id);
  END IF;
  RETURN NULL;
END;
$$;

CREATE OR REPLACE FUNCTION trg_activity_link_project_last_activity() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  IF TG_OP IN ('DELETE', 'UPDATE') THEN
    PERFORM move_project_last_activity(OLD.project_id);
  END IF;
  IF TG_OP IN ('INSERT', 'UPDATE') THEN
    PERFORM move_project_last_activity(NEW.project_id);
  END IF;
  RETURN NULL;
END;
$$;

DROP FUNCTION IF EXISTS advance_last_activity(regclass, uuid, timestamptz);
DROP FUNCTION IF EXISTS last_activity_of_activity(uuid);
