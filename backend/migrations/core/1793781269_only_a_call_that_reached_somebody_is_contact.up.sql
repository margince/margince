-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- The checks the previous migration added are validated here, under SHARE
-- UPDATE EXCLUSIVE, which readers and writers pass. Every row is NULL.
--
-- Only a call that reached somebody is contact (product decision, #8453):
-- Reached and Put through, and a call logged with no outcome. A voicemail, a
-- call back later and a call that reached nobody move no last-activity clock.
-- All four functions carry relstrength.LastActivityCountsSQL("a", "now()")
-- verbatim in every arm, held there by
-- backend/gates/lastactivitymeetingrule_test.go. Both clock triggers also fire
-- on call_outcome, which that rule now reads.
--
-- No stored clock needs a refold: every existing call has no outcome, which
-- the rule reads as before.
SET LOCAL lock_timeout = '3s';

-- One statement, so activity is scanned once for both.
ALTER TABLE activity
    VALIDATE CONSTRAINT activity_call_outcome_check,
    VALIDATE CONSTRAINT activity_call_outcome_kind_check;

CREATE OR REPLACE FUNCTION last_activity_of_deal(did uuid) RETURNS timestamptz
    LANGUAGE sql STABLE
    AS $$
  SELECT max(a.occurred_at)
    FROM activity_link l
    JOIN activity a ON a.id = l.activity_id AND a.archived_at IS NULL
     AND a.audience = 'workspace'
     AND a.origin NOT IN ('system_remediation', 'system_notice')
     AND (a.kind IN ('email','message','meeting','call','note') AND ((a.kind <> 'meeting' OR (a.meeting_status IS NULL OR a.meeting_status IN ('booked','held'))) AND (a.kind <> 'call' OR (a.call_outcome IS NULL OR a.call_outcome IN ('reached','put_through')))) AND ((a.kind = 'meeting' AND (a.occurred_at <= greatest(now(), ((date_trunc('day', CAST(now() AS timestamptz) AT TIME ZONE COALESCE((SELECT st.value #>> '{}' FROM setting st WHERE st.key = 'installation.timezone'), 'UTC')) + interval '12 hours') AT TIME ZONE 'UTC')) AND (a.occurred_precision = 'day' AND a.occurred_at < ((date_trunc('day', CAST(now() AS timestamptz) AT TIME ZONE COALESCE((SELECT st.value #>> '{}' FROM setting st WHERE st.key = 'installation.timezone'), 'UTC')) + interval '12 hours') AT TIME ZONE 'UTC') OR a.occurred_precision = 'instant' AND a.occurred_at <= now()))) OR (a.kind <> 'meeting' AND (a.occurred_at <= greatest(now(), ((date_trunc('day', CAST(now() AS timestamptz) AT TIME ZONE COALESCE((SELECT st.value #>> '{}' FROM setting st WHERE st.key = 'installation.timezone'), 'UTC')) + interval '12 hours') AT TIME ZONE 'UTC')) AND (a.occurred_precision = 'day' OR a.occurred_at <= now())))))
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
     AND (a.kind IN ('email','message','meeting','call','note') AND ((a.kind <> 'meeting' OR (a.meeting_status IS NULL OR a.meeting_status IN ('booked','held'))) AND (a.kind <> 'call' OR (a.call_outcome IS NULL OR a.call_outcome IN ('reached','put_through')))) AND ((a.kind = 'meeting' AND (a.occurred_at <= greatest(now(), ((date_trunc('day', CAST(now() AS timestamptz) AT TIME ZONE COALESCE((SELECT st.value #>> '{}' FROM setting st WHERE st.key = 'installation.timezone'), 'UTC')) + interval '12 hours') AT TIME ZONE 'UTC')) AND (a.occurred_precision = 'day' AND a.occurred_at < ((date_trunc('day', CAST(now() AS timestamptz) AT TIME ZONE COALESCE((SELECT st.value #>> '{}' FROM setting st WHERE st.key = 'installation.timezone'), 'UTC')) + interval '12 hours') AT TIME ZONE 'UTC') OR a.occurred_precision = 'instant' AND a.occurred_at <= now()))) OR (a.kind <> 'meeting' AND (a.occurred_at <= greatest(now(), ((date_trunc('day', CAST(now() AS timestamptz) AT TIME ZONE COALESCE((SELECT st.value #>> '{}' FROM setting st WHERE st.key = 'installation.timezone'), 'UTC')) + interval '12 hours') AT TIME ZONE 'UTC')) AND (a.occurred_precision = 'day' OR a.occurred_at <= now())))))
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
       AND (a.kind IN ('email','message','meeting','call','note') AND ((a.kind <> 'meeting' OR (a.meeting_status IS NULL OR a.meeting_status IN ('booked','held'))) AND (a.kind <> 'call' OR (a.call_outcome IS NULL OR a.call_outcome IN ('reached','put_through')))) AND ((a.kind = 'meeting' AND (a.occurred_at <= greatest(now(), ((date_trunc('day', CAST(now() AS timestamptz) AT TIME ZONE COALESCE((SELECT st.value #>> '{}' FROM setting st WHERE st.key = 'installation.timezone'), 'UTC')) + interval '12 hours') AT TIME ZONE 'UTC')) AND (a.occurred_precision = 'day' AND a.occurred_at < ((date_trunc('day', CAST(now() AS timestamptz) AT TIME ZONE COALESCE((SELECT st.value #>> '{}' FROM setting st WHERE st.key = 'installation.timezone'), 'UTC')) + interval '12 hours') AT TIME ZONE 'UTC') OR a.occurred_precision = 'instant' AND a.occurred_at <= now()))) OR (a.kind <> 'meeting' AND (a.occurred_at <= greatest(now(), ((date_trunc('day', CAST(now() AS timestamptz) AT TIME ZONE COALESCE((SELECT st.value #>> '{}' FROM setting st WHERE st.key = 'installation.timezone'), 'UTC')) + interval '12 hours') AT TIME ZONE 'UTC')) AND (a.occurred_precision = 'day' OR a.occurred_at <= now())))))
     WHERE l.company_id = cid
    UNION ALL
    -- Filed against one of its deals.
    SELECT max(a.occurred_at)
      FROM deal d
      JOIN activity_link l ON l.deal_id = d.id
      JOIN activity a ON a.id = l.activity_id AND a.archived_at IS NULL
       AND a.audience = 'workspace'
       AND a.origin NOT IN ('system_remediation', 'system_notice')
       AND (a.kind IN ('email','message','meeting','call','note') AND ((a.kind <> 'meeting' OR (a.meeting_status IS NULL OR a.meeting_status IN ('booked','held'))) AND (a.kind <> 'call' OR (a.call_outcome IS NULL OR a.call_outcome IN ('reached','put_through')))) AND ((a.kind = 'meeting' AND (a.occurred_at <= greatest(now(), ((date_trunc('day', CAST(now() AS timestamptz) AT TIME ZONE COALESCE((SELECT st.value #>> '{}' FROM setting st WHERE st.key = 'installation.timezone'), 'UTC')) + interval '12 hours') AT TIME ZONE 'UTC')) AND (a.occurred_precision = 'day' AND a.occurred_at < ((date_trunc('day', CAST(now() AS timestamptz) AT TIME ZONE COALESCE((SELECT st.value #>> '{}' FROM setting st WHERE st.key = 'installation.timezone'), 'UTC')) + interval '12 hours') AT TIME ZONE 'UTC') OR a.occurred_precision = 'instant' AND a.occurred_at <= now()))) OR (a.kind <> 'meeting' AND (a.occurred_at <= greatest(now(), ((date_trunc('day', CAST(now() AS timestamptz) AT TIME ZONE COALESCE((SELECT st.value #>> '{}' FROM setting st WHERE st.key = 'installation.timezone'), 'UTC')) + interval '12 hours') AT TIME ZONE 'UTC')) AND (a.occurred_precision = 'day' OR a.occurred_at <= now())))))
     WHERE d.company_id = cid
    UNION ALL
    -- Filed against a contact it currently employs, sent while they worked there.
    SELECT max(a.occurred_at)
      FROM relationship r
      JOIN activity_link l ON l.contact_id = r.contact_id
      JOIN activity a ON a.id = l.activity_id AND a.archived_at IS NULL
       AND a.audience = 'workspace'
       AND a.origin NOT IN ('system_remediation', 'system_notice')
       AND (a.kind IN ('email','message','meeting','call','note') AND ((a.kind <> 'meeting' OR (a.meeting_status IS NULL OR a.meeting_status IN ('booked','held'))) AND (a.kind <> 'call' OR (a.call_outcome IS NULL OR a.call_outcome IN ('reached','put_through')))) AND ((a.kind = 'meeting' AND (a.occurred_at <= greatest(now(), ((date_trunc('day', CAST(now() AS timestamptz) AT TIME ZONE COALESCE((SELECT st.value #>> '{}' FROM setting st WHERE st.key = 'installation.timezone'), 'UTC')) + interval '12 hours') AT TIME ZONE 'UTC')) AND (a.occurred_precision = 'day' AND a.occurred_at < ((date_trunc('day', CAST(now() AS timestamptz) AT TIME ZONE COALESCE((SELECT st.value #>> '{}' FROM setting st WHERE st.key = 'installation.timezone'), 'UTC')) + interval '12 hours') AT TIME ZONE 'UTC') OR a.occurred_precision = 'instant' AND a.occurred_at <= now()))) OR (a.kind <> 'meeting' AND (a.occurred_at <= greatest(now(), ((date_trunc('day', CAST(now() AS timestamptz) AT TIME ZONE COALESCE((SELECT st.value #>> '{}' FROM setting st WHERE st.key = 'installation.timezone'), 'UTC')) + interval '12 hours') AT TIME ZONE 'UTC')) AND (a.occurred_precision = 'day' OR a.occurred_at <= now())))))
     WHERE r.company_id = cid AND r.kind = 'employment'
       AND r.ended_at IS NULL AND r.archived_at IS NULL
       AND (CASE WHEN r.started_at IS NOT NULL THEN (r.started_at::timestamp AT TIME ZONE 'UTC' <= a.occurred_at)
		WHEN r.first_observed_at IS NOT NULL THEN (a.occurred_at >= r.first_observed_at AND (a.occurred_precision = 'instant' OR (CASE WHEN a.occurred_precision = 'day'
		THEN date_trunc('day', a.occurred_at AT TIME ZONE 'UTC') AT TIME ZONE COALESCE((SELECT st.value #>> '{}' FROM setting st WHERE st.key = 'installation.timezone'), 'UTC')
		ELSE a.occurred_at END) >= r.first_observed_at)) ELSE true END)
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
     AND (a.kind IN ('email','message','meeting','call','note') AND ((a.kind <> 'meeting' OR (a.meeting_status IS NULL OR a.meeting_status IN ('booked','held'))) AND (a.kind <> 'call' OR (a.call_outcome IS NULL OR a.call_outcome IN ('reached','put_through')))) AND ((a.kind = 'meeting' AND (a.occurred_at <= greatest(now(), ((date_trunc('day', CAST(now() AS timestamptz) AT TIME ZONE COALESCE((SELECT st.value #>> '{}' FROM setting st WHERE st.key = 'installation.timezone'), 'UTC')) + interval '12 hours') AT TIME ZONE 'UTC')) AND (a.occurred_precision = 'day' AND a.occurred_at < ((date_trunc('day', CAST(now() AS timestamptz) AT TIME ZONE COALESCE((SELECT st.value #>> '{}' FROM setting st WHERE st.key = 'installation.timezone'), 'UTC')) + interval '12 hours') AT TIME ZONE 'UTC') OR a.occurred_precision = 'instant' AND a.occurred_at <= now()))) OR (a.kind <> 'meeting' AND (a.occurred_at <= greatest(now(), ((date_trunc('day', CAST(now() AS timestamptz) AT TIME ZONE COALESCE((SELECT st.value #>> '{}' FROM setting st WHERE st.key = 'installation.timezone'), 'UTC')) + interval '12 hours') AT TIME ZONE 'UTC')) AND (a.occurred_precision = 'day' OR a.occurred_at <= now())))))
   WHERE l.project_id = pid
$$;

DROP TRIGGER activity_last_activity ON activity;
CREATE TRIGGER activity_last_activity
	AFTER UPDATE OF occurred_at, archived_at, audience, origin, kind, meeting_status, occurred_precision, call_outcome ON activity
	FOR EACH ROW
	WHEN (old.occurred_at IS DISTINCT FROM new.occurred_at
	   OR old.archived_at IS DISTINCT FROM new.archived_at
	   OR old.audience IS DISTINCT FROM new.audience
	   OR old.origin IS DISTINCT FROM new.origin
	   OR old.kind IS DISTINCT FROM new.kind
	   OR old.meeting_status IS DISTINCT FROM new.meeting_status
	   OR old.occurred_precision IS DISTINCT FROM new.occurred_precision
	   OR old.call_outcome IS DISTINCT FROM new.call_outcome)
	EXECUTE FUNCTION trg_activity_last_activity();

DROP TRIGGER activity_project_last_activity ON activity;
CREATE TRIGGER activity_project_last_activity
	AFTER UPDATE OF occurred_at, archived_at, audience, origin, kind, meeting_status, occurred_precision, call_outcome ON activity
	FOR EACH ROW
	WHEN (old.occurred_at IS DISTINCT FROM new.occurred_at
	   OR old.archived_at IS DISTINCT FROM new.archived_at
	   OR old.audience IS DISTINCT FROM new.audience
	   OR old.origin IS DISTINCT FROM new.origin
	   OR old.kind IS DISTINCT FROM new.kind
	   OR old.meeting_status IS DISTINCT FROM new.meeting_status
	   OR old.occurred_precision IS DISTINCT FROM new.occurred_precision
	   OR old.call_outcome IS DISTINCT FROM new.call_outcome)
	EXECUTE FUNCTION trg_activity_project_last_activity();
