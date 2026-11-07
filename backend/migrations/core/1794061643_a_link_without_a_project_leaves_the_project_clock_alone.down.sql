-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

CREATE OR REPLACE FUNCTION trg_activity_link_project_last_activity() RETURNS trigger
LANGUAGE plpgsql AS $$BEGIN
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
