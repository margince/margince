-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- A link moves only the clocks it reaches, so each insert arm asks for the
-- activity's time only when its link reaches one. PL/pgSQL evaluates a PERFORM's
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

-- A project-only or lead-only link reaches no contact, deal or company clock.
CREATE OR REPLACE FUNCTION trg_activity_link_last_activity() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
  at timestamptz;
  reached record;
BEGIN
  IF TG_OP = 'INSERT' THEN
    IF NEW.contact_id IS NULL AND NEW.deal_id IS NULL AND NEW.company_id IS NULL THEN
      RETURN NULL;
    END IF;
    at := last_activity_of_activity(NEW.activity_id);
    IF at IS NOT NULL THEN
      FOR reached IN
         SELECT reach.kind, reach.id
           FROM last_activity_reach(ARRAY[NEW.contact_id], ARRAY[NEW.deal_id], ARRAY[NEW.company_id]) reach
          ORDER BY reach.kind, reach.id
      LOOP
        PERFORM advance_last_activity(reached.kind::regclass, reached.id,
          CASE reached.kind WHEN 'company' THEN last_activity_of_company_activity(NEW.activity_id, reached.id) ELSE at END);
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
