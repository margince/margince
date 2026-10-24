-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- A task never moves last_activity_at on a deal, a contact, a company or a
-- project, whether a member or an agent filed it. A task is intent, not
-- contact: the commitment scan filing tasks under every record a mail reached
-- left all of them reading "Last activity: today".
--
-- All four functions carry relstrength.LastActivityCountsSQL("a", "now()")
-- verbatim in every arm, held there by
-- backend/gates/lastactivitymeetingrule_test.go. So the project clock also
-- stops counting a called-off meeting and one still ahead, as the other three
-- did already; the past_activity_clock_refold job now refolds projects too.
--
-- The project trigger learns kind and meeting_status, as activity_last_activity
-- did, so a re-kinded or called-off activity refolds its project. And
-- move_project_last_activity writes only a value that changed, as
-- move_last_activity does, so a refold that moves nothing rewrites nothing.
-- last_activity_reach and move_last_activity are unchanged.
--
-- The task_activity_clocks data task refolds every stored clock the old rules left.
SET LOCAL lock_timeout = '3s';

CREATE OR REPLACE FUNCTION last_activity_of_deal(did uuid) RETURNS timestamptz
    LANGUAGE sql STABLE
    AS $$
  SELECT max(a.occurred_at)
    FROM activity_link l
    JOIN activity a ON a.id = l.activity_id AND a.archived_at IS NULL
     AND a.audience = 'workspace'
     AND a.origin NOT IN ('system_remediation', 'system_notice')
     AND (a.kind IN ('email','message','meeting','call','note') AND (a.kind <> 'meeting' OR (a.meeting_status IS NULL OR a.meeting_status IN ('booked','held'))) AND a.occurred_at <= now())
   WHERE l.deal_id = did
$$;

CREATE OR REPLACE FUNCTION last_activity_of_contact(cid uuid) RETURNS timestamptz
    LANGUAGE sql STABLE
    AS $$
  SELECT max(a.occurred_at)
    FROM activity_link l
    JOIN activity a ON a.id = l.activity_id AND a.archived_at IS NULL
     AND a.audience = 'workspace'
     AND a.origin NOT IN ('system_remediation', 'system_notice')
     AND (a.kind IN ('email','message','meeting','call','note') AND (a.kind <> 'meeting' OR (a.meeting_status IS NULL OR a.meeting_status IN ('booked','held'))) AND a.occurred_at <= now())
   WHERE l.contact_id = cid
$$;

CREATE OR REPLACE FUNCTION last_activity_of_company(cid uuid) RETURNS timestamptz
    LANGUAGE sql STABLE
    AS $$
  SELECT max(v) FROM (
    -- Filed against the account itself.
    SELECT max(a.occurred_at) AS v
      FROM activity_link l
      JOIN activity a ON a.id = l.activity_id AND a.archived_at IS NULL
       AND a.audience = 'workspace'
       AND a.origin NOT IN ('system_remediation', 'system_notice')
       AND (a.kind IN ('email','message','meeting','call','note') AND (a.kind <> 'meeting' OR (a.meeting_status IS NULL OR a.meeting_status IN ('booked','held'))) AND a.occurred_at <= now())
     WHERE l.company_id = cid
    UNION ALL
    -- Filed against one of its deals.
    SELECT max(a.occurred_at)
      FROM deal d
      JOIN activity_link l ON l.deal_id = d.id
      JOIN activity a ON a.id = l.activity_id AND a.archived_at IS NULL
       AND a.audience = 'workspace'
       AND a.origin NOT IN ('system_remediation', 'system_notice')
       AND (a.kind IN ('email','message','meeting','call','note') AND (a.kind <> 'meeting' OR (a.meeting_status IS NULL OR a.meeting_status IN ('booked','held'))) AND a.occurred_at <= now())
     WHERE d.company_id = cid
    UNION ALL
    -- Filed against a contact it currently employs, sent while they worked there.
    SELECT max(a.occurred_at)
      FROM relationship r
      JOIN activity_link l ON l.contact_id = r.contact_id
      JOIN activity a ON a.id = l.activity_id AND a.archived_at IS NULL
       AND a.audience = 'workspace'
       AND a.origin NOT IN ('system_remediation', 'system_notice')
       AND (a.kind IN ('email','message','meeting','call','note') AND (a.kind <> 'meeting' OR (a.meeting_status IS NULL OR a.meeting_status IN ('booked','held'))) AND a.occurred_at <= now())
     WHERE r.company_id = cid AND r.kind = 'employment'
       AND r.ended_at IS NULL AND r.archived_at IS NULL
       AND coalesce(r.started_at::timestamp AT TIME ZONE 'UTC', r.first_observed_at, a.occurred_at) <= a.occurred_at
  ) arms
$$;

CREATE OR REPLACE FUNCTION last_activity_of_project(pid uuid) RETURNS timestamptz
    LANGUAGE sql STABLE
    AS $$
  SELECT max(a.occurred_at)
    FROM activity_link l
    JOIN activity a ON a.id = l.activity_id AND a.archived_at IS NULL
     AND a.audience = 'workspace'
     AND a.origin NOT IN ('system_remediation', 'system_notice')
     AND (a.kind IN ('email','message','meeting','call','note') AND (a.kind <> 'meeting' OR (a.meeting_status IS NULL OR a.meeting_status IN ('booked','held'))) AND a.occurred_at <= now())
   WHERE l.project_id = pid
$$;

CREATE OR REPLACE FUNCTION move_project_last_activity(pid uuid) RETURNS void
    LANGUAGE plpgsql
    AS $$
DECLARE
  v timestamptz;
BEGIN
  IF pid IS NULL THEN RETURN; END IF;
  PERFORM 1 FROM project WHERE id = pid FOR UPDATE;
  v := last_activity_of_project(pid);
  PERFORM set_config('margince.last_activity_move', 'on', true);
  UPDATE project SET last_activity_at = v WHERE id = pid AND last_activity_at IS DISTINCT FROM v;
  PERFORM set_config('margince.last_activity_move', 'off', true);
END;
$$;

DROP TRIGGER activity_project_last_activity ON activity;
CREATE TRIGGER activity_project_last_activity
	AFTER UPDATE OF occurred_at, archived_at, audience, origin, kind, meeting_status ON activity
	FOR EACH ROW
	WHEN (old.occurred_at IS DISTINCT FROM new.occurred_at
	   OR old.archived_at IS DISTINCT FROM new.archived_at
	   OR old.audience IS DISTINCT FROM new.audience
	   OR old.origin IS DISTINCT FROM new.origin
	   OR old.kind IS DISTINCT FROM new.kind
	   OR old.meeting_status IS DISTINCT FROM new.meeting_status)
	EXECUTE FUNCTION trg_activity_project_last_activity();
