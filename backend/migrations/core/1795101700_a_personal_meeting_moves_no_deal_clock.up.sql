-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- A deal's last-activity clock counts only activity that may speak for the
-- deal: relstrength.DealEvidenceSQL, carried verbatim in every arm and held
-- there by backend/gates/remediationnotbuyeractivity_test.go. A thread a seat
-- holds as personal, and a row whose audience reason is personal, move no deal
-- clock. The contact and company clocks are unchanged.
--
-- The deal clock also moves when a row's audience reason changes, and when a
-- seat's personal hold on a thread starts, ends or is deleted. Its status list
-- is threadverdict.HoldsSQL's. The personal_meetings_leave_the_deal_clock data
-- task refolds the stored deal clocks written before this.
SET LOCAL lock_timeout = '3s';

CREATE OR REPLACE FUNCTION last_activity_of_deal_activity(aid uuid) RETURNS timestamptz
LANGUAGE sql STABLE AS $$SELECT max(a.occurred_at)
    FROM activity_link l
    JOIN activity a ON a.id = l.activity_id AND a.archived_at IS NULL
     AND a.audience IN ('workspace', 'participants')
     AND (coalesce(a.audience_reason, '') <> 'personal' AND NOT EXISTS (
 SELECT 1 FROM capture_import thread_hold_import
 JOIN capture_thread_verdict thread_hold ON thread_hold.user_id=thread_hold_import.user_id
 AND thread_hold.thread_key=a.thread_key
 WHERE thread_hold_import.activity_id=a.id AND thread_hold.kind = 'personal' AND thread_hold.status IN ('held', 'held_by_owner', 'unsure')))
     AND a.origin NOT IN ('system_remediation', 'system_notice')
     AND (a.kind IN ('email','message','meeting','call','note') AND ((a.kind <> 'meeting' OR ((a.meeting_status IS NULL OR a.meeting_status IN ('booked','held')) AND (a.occurred_at <= greatest(now(), ((date_trunc('day', CAST(now() AS timestamptz) AT TIME ZONE COALESCE((SELECT st.value #>> '{}' FROM setting st WHERE st.key = 'installation.timezone'), 'UTC')) + interval '12 hours') AT TIME ZONE 'UTC')) AND (a.occurred_precision = 'day' AND a.occurred_at < ((date_trunc('day', CAST(now() AS timestamptz) AT TIME ZONE COALESCE((SELECT st.value #>> '{}' FROM setting st WHERE st.key = 'installation.timezone'), 'UTC')) + interval '12 hours') AT TIME ZONE 'UTC') OR a.occurred_precision = 'instant' AND a.occurred_at <= now())))) AND (a.kind <> 'call' OR (a.call_outcome IS NULL OR a.call_outcome IN ('reached','put_through')))) AND NOT (a.kind = 'note' AND a.origin = 'agent') AND NOT (a.kind = 'email' AND a.bulk_mail_attested) AND ((a.kind = 'meeting' AND (a.occurred_at <= greatest(now(), ((date_trunc('day', CAST(now() AS timestamptz) AT TIME ZONE COALESCE((SELECT st.value #>> '{}' FROM setting st WHERE st.key = 'installation.timezone'), 'UTC')) + interval '12 hours') AT TIME ZONE 'UTC')) AND (a.occurred_precision = 'day' AND a.occurred_at < ((date_trunc('day', CAST(now() AS timestamptz) AT TIME ZONE COALESCE((SELECT st.value #>> '{}' FROM setting st WHERE st.key = 'installation.timezone'), 'UTC')) + interval '12 hours') AT TIME ZONE 'UTC') OR a.occurred_precision = 'instant' AND a.occurred_at <= now()))) OR (a.kind <> 'meeting' AND (a.occurred_at <= greatest(now(), ((date_trunc('day', CAST(now() AS timestamptz) AT TIME ZONE COALESCE((SELECT st.value #>> '{}' FROM setting st WHERE st.key = 'installation.timezone'), 'UTC')) + interval '12 hours') AT TIME ZONE 'UTC')) AND (a.occurred_precision = 'day' OR a.occurred_at <= now())))))
   WHERE l.activity_id = aid
$$;

CREATE OR REPLACE FUNCTION last_activity_of_deal(did uuid) RETURNS timestamptz
    LANGUAGE sql STABLE
    AS $$
  SELECT max(a.occurred_at)
    FROM activity_link l
    JOIN activity a ON a.id = l.activity_id AND a.archived_at IS NULL
     AND a.audience IN ('workspace', 'participants')
     AND (coalesce(a.audience_reason, '') <> 'personal' AND NOT EXISTS (
 SELECT 1 FROM capture_import thread_hold_import
 JOIN capture_thread_verdict thread_hold ON thread_hold.user_id=thread_hold_import.user_id
 AND thread_hold.thread_key=a.thread_key
 WHERE thread_hold_import.activity_id=a.id AND thread_hold.kind = 'personal' AND thread_hold.status IN ('held', 'held_by_owner', 'unsure')))
     AND a.origin NOT IN ('system_remediation', 'system_notice')
     AND (a.kind IN ('email','message','meeting','call','note') AND ((a.kind <> 'meeting' OR ((a.meeting_status IS NULL OR a.meeting_status IN ('booked','held')) AND (a.occurred_at <= greatest(now(), ((date_trunc('day', CAST(now() AS timestamptz) AT TIME ZONE COALESCE((SELECT st.value #>> '{}' FROM setting st WHERE st.key = 'installation.timezone'), 'UTC')) + interval '12 hours') AT TIME ZONE 'UTC')) AND (a.occurred_precision = 'day' AND a.occurred_at < ((date_trunc('day', CAST(now() AS timestamptz) AT TIME ZONE COALESCE((SELECT st.value #>> '{}' FROM setting st WHERE st.key = 'installation.timezone'), 'UTC')) + interval '12 hours') AT TIME ZONE 'UTC') OR a.occurred_precision = 'instant' AND a.occurred_at <= now())))) AND (a.kind <> 'call' OR (a.call_outcome IS NULL OR a.call_outcome IN ('reached','put_through')))) AND NOT (a.kind = 'note' AND a.origin = 'agent') AND NOT (a.kind = 'email' AND a.bulk_mail_attested) AND ((a.kind = 'meeting' AND (a.occurred_at <= greatest(now(), ((date_trunc('day', CAST(now() AS timestamptz) AT TIME ZONE COALESCE((SELECT st.value #>> '{}' FROM setting st WHERE st.key = 'installation.timezone'), 'UTC')) + interval '12 hours') AT TIME ZONE 'UTC')) AND (a.occurred_precision = 'day' AND a.occurred_at < ((date_trunc('day', CAST(now() AS timestamptz) AT TIME ZONE COALESCE((SELECT st.value #>> '{}' FROM setting st WHERE st.key = 'installation.timezone'), 'UTC')) + interval '12 hours') AT TIME ZONE 'UTC') OR a.occurred_precision = 'instant' AND a.occurred_at <= now()))) OR (a.kind <> 'meeting' AND (a.occurred_at <= greatest(now(), ((date_trunc('day', CAST(now() AS timestamptz) AT TIME ZONE COALESCE((SELECT st.value #>> '{}' FROM setting st WHERE st.key = 'installation.timezone'), 'UTC')) + interval '12 hours') AT TIME ZONE 'UTC')) AND (a.occurred_precision = 'day' OR a.occurred_at <= now())))))
   WHERE l.deal_id = did
$$;

DROP TRIGGER activity_last_activity ON activity;
CREATE TRIGGER activity_last_activity
	AFTER UPDATE OF occurred_at, archived_at, audience, audience_reason, origin, kind, meeting_status, occurred_precision, call_outcome, bulk_mail_attested ON activity
	FOR EACH ROW
	WHEN (old.occurred_at IS DISTINCT FROM new.occurred_at
	   OR old.archived_at IS DISTINCT FROM new.archived_at
	   OR old.audience IS DISTINCT FROM new.audience
	   OR old.audience_reason IS DISTINCT FROM new.audience_reason
	   OR old.origin IS DISTINCT FROM new.origin
	   OR old.kind IS DISTINCT FROM new.kind
	   OR old.meeting_status IS DISTINCT FROM new.meeting_status
	   OR old.occurred_precision IS DISTINCT FROM new.occurred_precision
	   OR old.call_outcome IS DISTINCT FROM new.call_outcome
	   OR old.bulk_mail_attested IS DISTINCT FROM new.bulk_mail_attested)
	EXECUTE FUNCTION trg_activity_last_activity();

CREATE FUNCTION refresh_deal_clocks_of_thread(key text, seat uuid) RETURNS void
LANGUAGE plpgsql AS $$
DECLARE
  l record;
BEGIN
  FOR l IN
     SELECT DISTINCT link.deal_id FROM activity a
       JOIN capture_import i ON i.activity_id = a.id AND i.user_id = seat
       JOIN activity_link link ON link.activity_id = a.id
      WHERE a.thread_key = key AND key <> '' AND link.deal_id IS NOT NULL
      ORDER BY link.deal_id
  LOOP
    PERFORM refresh_last_activity_for_link(NULL, l.deal_id, NULL);
  END LOOP;
END;
$$;

CREATE FUNCTION trg_thread_verdict_deal_clock() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
  was_personal boolean := false;
  is_personal boolean := false;
BEGIN
  IF TG_OP <> 'INSERT' THEN
    was_personal := OLD.kind = 'personal' AND OLD.status IN ('held', 'held_by_owner', 'unsure');
  END IF;
  IF TG_OP <> 'DELETE' THEN
    is_personal := NEW.kind = 'personal' AND NEW.status IN ('held', 'held_by_owner', 'unsure');
  END IF;
  IF TG_OP = 'UPDATE' AND was_personal = is_personal
     AND OLD.thread_key = NEW.thread_key AND OLD.user_id = NEW.user_id THEN
    RETURN NULL;
  END IF;
  IF was_personal THEN
    PERFORM refresh_deal_clocks_of_thread(OLD.thread_key, OLD.user_id);
  END IF;
  IF is_personal THEN
    PERFORM refresh_deal_clocks_of_thread(NEW.thread_key, NEW.user_id);
  END IF;
  RETURN NULL;
END;
$$;

CREATE TRIGGER capture_thread_verdict_deal_clock
	AFTER INSERT OR UPDATE OF kind, status, thread_key, user_id OR DELETE ON capture_thread_verdict
	FOR EACH ROW
	EXECUTE FUNCTION trg_thread_verdict_deal_clock();
