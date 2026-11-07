-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- A link without a project moves no project clock, so the insert arm asks for
-- the activity's time only when there is one. PL/pgSQL evaluates a PERFORM's
-- arguments before the call, so advance_last_activity returning on a NULL id
-- still paid for last_activity_of_activity on every contact link.
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
