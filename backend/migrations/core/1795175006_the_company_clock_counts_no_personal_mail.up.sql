-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- The company clock counts no personal mail. Every arm of
-- last_activity_of_company and last_activity_of_company_activity carries
-- relstrength.NotPersonalSQL, so a thread a seat holds as personal, or a row
-- whose audience reason is personal, moves no company clock. The arm through a
-- deal carries relstrength.DealEvidenceSQL, as the deal clock does, so a link
-- judged not about the deal reaches the company only through its other links.
-- backend/gates/remediationnotbuyeractivity_test.go holds the clauses.
--
-- A seat's personal hold starting or ending now refreshes the company clocks
-- the thread reaches, not only its deal clocks. So does a seat's import of a
-- message on a thread it holds, and a row changing thread. The
-- company_clock_counts_no_personal_mail data task refolds the stored clocks.
SET LOCAL lock_timeout = '3s';

CREATE OR REPLACE FUNCTION last_activity_of_company(cid uuid) RETURNS timestamptz
    LANGUAGE sql STABLE
    AS $$
  SELECT max(v) FROM (
    -- Filed against the account itself.
    SELECT max(a.occurred_at) AS v
      FROM activity_link l
      JOIN activity a ON a.id = l.activity_id AND a.archived_at IS NULL
       AND a.audience = 'workspace'
       AND (coalesce(a.audience_reason, '') <> 'personal' AND NOT EXISTS (
 SELECT 1 FROM capture_import thread_hold_import
 JOIN capture_thread_verdict thread_hold ON thread_hold.user_id=thread_hold_import.user_id
 AND thread_hold.thread_key=a.thread_key
 WHERE thread_hold_import.activity_id=a.id AND thread_hold.kind = 'personal' AND thread_hold.status IN ('held', 'held_by_owner', 'unsure')))
       AND a.origin NOT IN ('system_remediation', 'system_notice')
       AND (a.kind IN ('email','message','meeting','call','note') AND ((a.kind <> 'meeting' OR ((a.meeting_status IS NULL OR a.meeting_status IN ('booked','held')) AND (a.occurred_at <= greatest(now(), ((date_trunc('day', CAST(now() AS timestamptz) AT TIME ZONE COALESCE((SELECT st.value #>> '{}' FROM setting st WHERE st.key = 'installation.timezone'), 'UTC')) + interval '12 hours') AT TIME ZONE 'UTC')) AND (a.occurred_precision = 'day' AND a.occurred_at < ((date_trunc('day', CAST(now() AS timestamptz) AT TIME ZONE COALESCE((SELECT st.value #>> '{}' FROM setting st WHERE st.key = 'installation.timezone'), 'UTC')) + interval '12 hours') AT TIME ZONE 'UTC') OR a.occurred_precision = 'instant' AND a.occurred_at <= now())))) AND (a.kind <> 'call' OR (a.call_outcome IS NULL OR a.call_outcome IN ('reached','put_through')))) AND NOT (a.kind = 'note' AND a.origin = 'agent') AND NOT (a.kind = 'email' AND a.bulk_mail_attested) AND ((a.kind = 'meeting' AND (a.occurred_at <= greatest(now(), ((date_trunc('day', CAST(now() AS timestamptz) AT TIME ZONE COALESCE((SELECT st.value #>> '{}' FROM setting st WHERE st.key = 'installation.timezone'), 'UTC')) + interval '12 hours') AT TIME ZONE 'UTC')) AND (a.occurred_precision = 'day' AND a.occurred_at < ((date_trunc('day', CAST(now() AS timestamptz) AT TIME ZONE COALESCE((SELECT st.value #>> '{}' FROM setting st WHERE st.key = 'installation.timezone'), 'UTC')) + interval '12 hours') AT TIME ZONE 'UTC') OR a.occurred_precision = 'instant' AND a.occurred_at <= now()))) OR (a.kind <> 'meeting' AND (a.occurred_at <= greatest(now(), ((date_trunc('day', CAST(now() AS timestamptz) AT TIME ZONE COALESCE((SELECT st.value #>> '{}' FROM setting st WHERE st.key = 'installation.timezone'), 'UTC')) + interval '12 hours') AT TIME ZONE 'UTC')) AND (a.occurred_precision = 'day' OR a.occurred_at <= now())))))
     WHERE l.company_id = cid
    UNION ALL
    -- Filed against one of its deals.
    SELECT max(a.occurred_at)
      FROM deal d
      JOIN activity_link l ON l.deal_id = d.id
      JOIN activity a ON a.id = l.activity_id AND a.archived_at IS NULL
       AND a.audience = 'workspace'
       AND ((coalesce(a.audience_reason, '') <> 'personal' AND NOT EXISTS (
 SELECT 1 FROM capture_import thread_hold_import
 JOIN capture_thread_verdict thread_hold ON thread_hold.user_id=thread_hold_import.user_id
 AND thread_hold.thread_key=a.thread_key
 WHERE thread_hold_import.activity_id=a.id AND thread_hold.kind = 'personal' AND thread_hold.status IN ('held', 'held_by_owner', 'unsure'))) AND NOT EXISTS (SELECT 1 FROM activity_link judged WHERE judged.activity_id = a.id AND judged.deal_id = l.deal_id AND judged.deal_relevance IN ('unrelated', 'uncertain')))
       AND a.origin NOT IN ('system_remediation', 'system_notice')
       AND (a.kind IN ('email','message','meeting','call','note') AND ((a.kind <> 'meeting' OR ((a.meeting_status IS NULL OR a.meeting_status IN ('booked','held')) AND (a.occurred_at <= greatest(now(), ((date_trunc('day', CAST(now() AS timestamptz) AT TIME ZONE COALESCE((SELECT st.value #>> '{}' FROM setting st WHERE st.key = 'installation.timezone'), 'UTC')) + interval '12 hours') AT TIME ZONE 'UTC')) AND (a.occurred_precision = 'day' AND a.occurred_at < ((date_trunc('day', CAST(now() AS timestamptz) AT TIME ZONE COALESCE((SELECT st.value #>> '{}' FROM setting st WHERE st.key = 'installation.timezone'), 'UTC')) + interval '12 hours') AT TIME ZONE 'UTC') OR a.occurred_precision = 'instant' AND a.occurred_at <= now())))) AND (a.kind <> 'call' OR (a.call_outcome IS NULL OR a.call_outcome IN ('reached','put_through')))) AND NOT (a.kind = 'note' AND a.origin = 'agent') AND NOT (a.kind = 'email' AND a.bulk_mail_attested) AND ((a.kind = 'meeting' AND (a.occurred_at <= greatest(now(), ((date_trunc('day', CAST(now() AS timestamptz) AT TIME ZONE COALESCE((SELECT st.value #>> '{}' FROM setting st WHERE st.key = 'installation.timezone'), 'UTC')) + interval '12 hours') AT TIME ZONE 'UTC')) AND (a.occurred_precision = 'day' AND a.occurred_at < ((date_trunc('day', CAST(now() AS timestamptz) AT TIME ZONE COALESCE((SELECT st.value #>> '{}' FROM setting st WHERE st.key = 'installation.timezone'), 'UTC')) + interval '12 hours') AT TIME ZONE 'UTC') OR a.occurred_precision = 'instant' AND a.occurred_at <= now()))) OR (a.kind <> 'meeting' AND (a.occurred_at <= greatest(now(), ((date_trunc('day', CAST(now() AS timestamptz) AT TIME ZONE COALESCE((SELECT st.value #>> '{}' FROM setting st WHERE st.key = 'installation.timezone'), 'UTC')) + interval '12 hours') AT TIME ZONE 'UTC')) AND (a.occurred_precision = 'day' OR a.occurred_at <= now())))))
     WHERE d.company_id = cid
    UNION ALL
    -- Filed against a contact it currently employs, sent while they worked there.
    SELECT max(a.occurred_at)
      FROM relationship r
      JOIN activity_link l ON l.contact_id = r.contact_id
      JOIN activity a ON a.id = l.activity_id AND a.archived_at IS NULL
       AND a.audience = 'workspace'
       AND (coalesce(a.audience_reason, '') <> 'personal' AND NOT EXISTS (
 SELECT 1 FROM capture_import thread_hold_import
 JOIN capture_thread_verdict thread_hold ON thread_hold.user_id=thread_hold_import.user_id
 AND thread_hold.thread_key=a.thread_key
 WHERE thread_hold_import.activity_id=a.id AND thread_hold.kind = 'personal' AND thread_hold.status IN ('held', 'held_by_owner', 'unsure')))
       AND a.origin NOT IN ('system_remediation', 'system_notice')
       AND (a.kind IN ('email','message','meeting','call','note') AND ((a.kind <> 'meeting' OR ((a.meeting_status IS NULL OR a.meeting_status IN ('booked','held')) AND (a.occurred_at <= greatest(now(), ((date_trunc('day', CAST(now() AS timestamptz) AT TIME ZONE COALESCE((SELECT st.value #>> '{}' FROM setting st WHERE st.key = 'installation.timezone'), 'UTC')) + interval '12 hours') AT TIME ZONE 'UTC')) AND (a.occurred_precision = 'day' AND a.occurred_at < ((date_trunc('day', CAST(now() AS timestamptz) AT TIME ZONE COALESCE((SELECT st.value #>> '{}' FROM setting st WHERE st.key = 'installation.timezone'), 'UTC')) + interval '12 hours') AT TIME ZONE 'UTC') OR a.occurred_precision = 'instant' AND a.occurred_at <= now())))) AND (a.kind <> 'call' OR (a.call_outcome IS NULL OR a.call_outcome IN ('reached','put_through')))) AND NOT (a.kind = 'note' AND a.origin = 'agent') AND NOT (a.kind = 'email' AND a.bulk_mail_attested) AND ((a.kind = 'meeting' AND (a.occurred_at <= greatest(now(), ((date_trunc('day', CAST(now() AS timestamptz) AT TIME ZONE COALESCE((SELECT st.value #>> '{}' FROM setting st WHERE st.key = 'installation.timezone'), 'UTC')) + interval '12 hours') AT TIME ZONE 'UTC')) AND (a.occurred_precision = 'day' AND a.occurred_at < ((date_trunc('day', CAST(now() AS timestamptz) AT TIME ZONE COALESCE((SELECT st.value #>> '{}' FROM setting st WHERE st.key = 'installation.timezone'), 'UTC')) + interval '12 hours') AT TIME ZONE 'UTC') OR a.occurred_precision = 'instant' AND a.occurred_at <= now()))) OR (a.kind <> 'meeting' AND (a.occurred_at <= greatest(now(), ((date_trunc('day', CAST(now() AS timestamptz) AT TIME ZONE COALESCE((SELECT st.value #>> '{}' FROM setting st WHERE st.key = 'installation.timezone'), 'UTC')) + interval '12 hours') AT TIME ZONE 'UTC')) AND (a.occurred_precision = 'day' OR a.occurred_at <= now())))))
     WHERE r.company_id = cid
       AND r.kind = 'employment' AND r.archived_at IS NULL AND (coalesce(r.employment_status, 'current') = 'current' AND (r.ended_at IS NULL OR CASE WHEN r.ended_precision = 'month' THEN (date_trunc('month', r.ended_at::date) + interval '1 month')::date ELSE r.ended_at END > (CAST(now() AS timestamptz) AT TIME ZONE COALESCE((SELECT st.value #>> '{}' FROM setting st WHERE st.key = 'installation.timezone'), 'UTC'))::date))
       AND (CASE WHEN r.started_at IS NOT NULL THEN (r.started_at::timestamp AT TIME ZONE 'UTC' <= a.occurred_at)
		WHEN r.first_observed_at IS NOT NULL THEN (a.occurred_at >= r.first_observed_at AND (a.occurred_precision = 'instant' OR (CASE WHEN a.occurred_precision = 'day'
		THEN date_trunc('day', a.occurred_at AT TIME ZONE 'UTC') AT TIME ZONE COALESCE((SELECT st.value #>> '{}' FROM setting st WHERE st.key = 'installation.timezone'), 'UTC')
		ELSE a.occurred_at END) >= r.first_observed_at)) ELSE true END)
  ) arms
$$;

CREATE OR REPLACE FUNCTION last_activity_of_company_activity(aid uuid, oid uuid) RETURNS timestamptz
LANGUAGE sql STABLE AS $$SELECT max(v) FROM (
    -- Filed against the account itself.
    SELECT max(a.occurred_at) AS v
      FROM activity_link l
      JOIN activity a ON a.id = l.activity_id AND a.archived_at IS NULL
       AND a.audience = 'workspace'
       AND (coalesce(a.audience_reason, '') <> 'personal' AND NOT EXISTS (
 SELECT 1 FROM capture_import thread_hold_import
 JOIN capture_thread_verdict thread_hold ON thread_hold.user_id=thread_hold_import.user_id
 AND thread_hold.thread_key=a.thread_key
 WHERE thread_hold_import.activity_id=a.id AND thread_hold.kind = 'personal' AND thread_hold.status IN ('held', 'held_by_owner', 'unsure')))
       AND a.origin NOT IN ('system_remediation', 'system_notice')
       AND (a.kind IN ('email','message','meeting','call','note') AND ((a.kind <> 'meeting' OR ((a.meeting_status IS NULL OR a.meeting_status IN ('booked','held')) AND (a.occurred_at <= greatest(now(), ((date_trunc('day', CAST(now() AS timestamptz) AT TIME ZONE COALESCE((SELECT st.value #>> '{}' FROM setting st WHERE st.key = 'installation.timezone'), 'UTC')) + interval '12 hours') AT TIME ZONE 'UTC')) AND (a.occurred_precision = 'day' AND a.occurred_at < ((date_trunc('day', CAST(now() AS timestamptz) AT TIME ZONE COALESCE((SELECT st.value #>> '{}' FROM setting st WHERE st.key = 'installation.timezone'), 'UTC')) + interval '12 hours') AT TIME ZONE 'UTC') OR a.occurred_precision = 'instant' AND a.occurred_at <= now())))) AND (a.kind <> 'call' OR (a.call_outcome IS NULL OR a.call_outcome IN ('reached','put_through')))) AND NOT (a.kind = 'note' AND a.origin = 'agent') AND NOT (a.kind = 'email' AND a.bulk_mail_attested) AND ((a.kind = 'meeting' AND (a.occurred_at <= greatest(now(), ((date_trunc('day', CAST(now() AS timestamptz) AT TIME ZONE COALESCE((SELECT st.value #>> '{}' FROM setting st WHERE st.key = 'installation.timezone'), 'UTC')) + interval '12 hours') AT TIME ZONE 'UTC')) AND (a.occurred_precision = 'day' AND a.occurred_at < ((date_trunc('day', CAST(now() AS timestamptz) AT TIME ZONE COALESCE((SELECT st.value #>> '{}' FROM setting st WHERE st.key = 'installation.timezone'), 'UTC')) + interval '12 hours') AT TIME ZONE 'UTC') OR a.occurred_precision = 'instant' AND a.occurred_at <= now()))) OR (a.kind <> 'meeting' AND (a.occurred_at <= greatest(now(), ((date_trunc('day', CAST(now() AS timestamptz) AT TIME ZONE COALESCE((SELECT st.value #>> '{}' FROM setting st WHERE st.key = 'installation.timezone'), 'UTC')) + interval '12 hours') AT TIME ZONE 'UTC')) AND (a.occurred_precision = 'day' OR a.occurred_at <= now())))))
     WHERE l.activity_id = aid AND l.company_id = oid
    UNION ALL
    -- Filed against one of its deals.
    SELECT max(a.occurred_at)
      FROM deal d
      JOIN activity_link l ON l.deal_id = d.id
      JOIN activity a ON a.id = l.activity_id AND a.archived_at IS NULL
       AND a.audience = 'workspace'
       AND ((coalesce(a.audience_reason, '') <> 'personal' AND NOT EXISTS (
 SELECT 1 FROM capture_import thread_hold_import
 JOIN capture_thread_verdict thread_hold ON thread_hold.user_id=thread_hold_import.user_id
 AND thread_hold.thread_key=a.thread_key
 WHERE thread_hold_import.activity_id=a.id AND thread_hold.kind = 'personal' AND thread_hold.status IN ('held', 'held_by_owner', 'unsure'))) AND NOT EXISTS (SELECT 1 FROM activity_link judged WHERE judged.activity_id = a.id AND judged.deal_id = l.deal_id AND judged.deal_relevance IN ('unrelated', 'uncertain')))
       AND a.origin NOT IN ('system_remediation', 'system_notice')
       AND (a.kind IN ('email','message','meeting','call','note') AND ((a.kind <> 'meeting' OR ((a.meeting_status IS NULL OR a.meeting_status IN ('booked','held')) AND (a.occurred_at <= greatest(now(), ((date_trunc('day', CAST(now() AS timestamptz) AT TIME ZONE COALESCE((SELECT st.value #>> '{}' FROM setting st WHERE st.key = 'installation.timezone'), 'UTC')) + interval '12 hours') AT TIME ZONE 'UTC')) AND (a.occurred_precision = 'day' AND a.occurred_at < ((date_trunc('day', CAST(now() AS timestamptz) AT TIME ZONE COALESCE((SELECT st.value #>> '{}' FROM setting st WHERE st.key = 'installation.timezone'), 'UTC')) + interval '12 hours') AT TIME ZONE 'UTC') OR a.occurred_precision = 'instant' AND a.occurred_at <= now())))) AND (a.kind <> 'call' OR (a.call_outcome IS NULL OR a.call_outcome IN ('reached','put_through')))) AND NOT (a.kind = 'note' AND a.origin = 'agent') AND NOT (a.kind = 'email' AND a.bulk_mail_attested) AND ((a.kind = 'meeting' AND (a.occurred_at <= greatest(now(), ((date_trunc('day', CAST(now() AS timestamptz) AT TIME ZONE COALESCE((SELECT st.value #>> '{}' FROM setting st WHERE st.key = 'installation.timezone'), 'UTC')) + interval '12 hours') AT TIME ZONE 'UTC')) AND (a.occurred_precision = 'day' AND a.occurred_at < ((date_trunc('day', CAST(now() AS timestamptz) AT TIME ZONE COALESCE((SELECT st.value #>> '{}' FROM setting st WHERE st.key = 'installation.timezone'), 'UTC')) + interval '12 hours') AT TIME ZONE 'UTC') OR a.occurred_precision = 'instant' AND a.occurred_at <= now()))) OR (a.kind <> 'meeting' AND (a.occurred_at <= greatest(now(), ((date_trunc('day', CAST(now() AS timestamptz) AT TIME ZONE COALESCE((SELECT st.value #>> '{}' FROM setting st WHERE st.key = 'installation.timezone'), 'UTC')) + interval '12 hours') AT TIME ZONE 'UTC')) AND (a.occurred_precision = 'day' OR a.occurred_at <= now())))))
     WHERE l.activity_id = aid AND d.company_id = oid
    UNION ALL
    -- Filed against a contact it currently employs, sent while they worked there.
    SELECT max(a.occurred_at)
      FROM relationship r
      JOIN activity_link l ON l.contact_id = r.contact_id
      JOIN activity a ON a.id = l.activity_id AND a.archived_at IS NULL
       AND a.audience = 'workspace'
       AND (coalesce(a.audience_reason, '') <> 'personal' AND NOT EXISTS (
 SELECT 1 FROM capture_import thread_hold_import
 JOIN capture_thread_verdict thread_hold ON thread_hold.user_id=thread_hold_import.user_id
 AND thread_hold.thread_key=a.thread_key
 WHERE thread_hold_import.activity_id=a.id AND thread_hold.kind = 'personal' AND thread_hold.status IN ('held', 'held_by_owner', 'unsure')))
       AND a.origin NOT IN ('system_remediation', 'system_notice')
       AND (a.kind IN ('email','message','meeting','call','note') AND ((a.kind <> 'meeting' OR ((a.meeting_status IS NULL OR a.meeting_status IN ('booked','held')) AND (a.occurred_at <= greatest(now(), ((date_trunc('day', CAST(now() AS timestamptz) AT TIME ZONE COALESCE((SELECT st.value #>> '{}' FROM setting st WHERE st.key = 'installation.timezone'), 'UTC')) + interval '12 hours') AT TIME ZONE 'UTC')) AND (a.occurred_precision = 'day' AND a.occurred_at < ((date_trunc('day', CAST(now() AS timestamptz) AT TIME ZONE COALESCE((SELECT st.value #>> '{}' FROM setting st WHERE st.key = 'installation.timezone'), 'UTC')) + interval '12 hours') AT TIME ZONE 'UTC') OR a.occurred_precision = 'instant' AND a.occurred_at <= now())))) AND (a.kind <> 'call' OR (a.call_outcome IS NULL OR a.call_outcome IN ('reached','put_through')))) AND NOT (a.kind = 'note' AND a.origin = 'agent') AND NOT (a.kind = 'email' AND a.bulk_mail_attested) AND ((a.kind = 'meeting' AND (a.occurred_at <= greatest(now(), ((date_trunc('day', CAST(now() AS timestamptz) AT TIME ZONE COALESCE((SELECT st.value #>> '{}' FROM setting st WHERE st.key = 'installation.timezone'), 'UTC')) + interval '12 hours') AT TIME ZONE 'UTC')) AND (a.occurred_precision = 'day' AND a.occurred_at < ((date_trunc('day', CAST(now() AS timestamptz) AT TIME ZONE COALESCE((SELECT st.value #>> '{}' FROM setting st WHERE st.key = 'installation.timezone'), 'UTC')) + interval '12 hours') AT TIME ZONE 'UTC') OR a.occurred_precision = 'instant' AND a.occurred_at <= now()))) OR (a.kind <> 'meeting' AND (a.occurred_at <= greatest(now(), ((date_trunc('day', CAST(now() AS timestamptz) AT TIME ZONE COALESCE((SELECT st.value #>> '{}' FROM setting st WHERE st.key = 'installation.timezone'), 'UTC')) + interval '12 hours') AT TIME ZONE 'UTC')) AND (a.occurred_precision = 'day' OR a.occurred_at <= now())))))
     WHERE l.activity_id = aid AND r.company_id = oid
       AND r.kind = 'employment' AND r.archived_at IS NULL AND (coalesce(r.employment_status, 'current') = 'current' AND (r.ended_at IS NULL OR CASE WHEN r.ended_precision = 'month' THEN (date_trunc('month', r.ended_at::date) + interval '1 month')::date ELSE r.ended_at END > (CAST(now() AS timestamptz) AT TIME ZONE COALESCE((SELECT st.value #>> '{}' FROM setting st WHERE st.key = 'installation.timezone'), 'UTC'))::date))
       AND (CASE WHEN r.started_at IS NOT NULL THEN (r.started_at::timestamp AT TIME ZONE 'UTC' <= a.occurred_at)
		WHEN r.first_observed_at IS NOT NULL THEN (a.occurred_at >= r.first_observed_at AND (a.occurred_precision = 'instant' OR (CASE WHEN a.occurred_precision = 'day'
		THEN date_trunc('day', a.occurred_at AT TIME ZONE 'UTC') AT TIME ZONE COALESCE((SELECT st.value #>> '{}' FROM setting st WHERE st.key = 'installation.timezone'), 'UTC')
		ELSE a.occurred_at END) >= r.first_observed_at)) ELSE true END)
  ) arms
$$;

CREATE FUNCTION refresh_clocks_of_thread(key text, seat uuid) RETURNS void
LANGUAGE plpgsql AS $$
DECLARE
  reached record;
BEGIN
  FOR reached IN
     WITH links AS (
       SELECT link.contact_id, link.deal_id, link.company_id FROM activity a
         JOIN capture_import i ON i.activity_id = a.id AND i.user_id = seat
         JOIN activity_link link ON link.activity_id = a.id
        WHERE a.thread_key = key AND key <> ''
     )
     SELECT reach.kind, reach.id FROM last_activity_reach(
         ARRAY(SELECT contact_id FROM links WHERE contact_id IS NOT NULL),
         ARRAY(SELECT deal_id FROM links WHERE deal_id IS NOT NULL),
         ARRAY(SELECT company_id FROM links WHERE company_id IS NOT NULL)) reach
      WHERE reach.kind IN ('company', 'deal')
      ORDER BY reach.kind, reach.id
  LOOP
    PERFORM move_last_activity(reached.kind::regclass, reached.id);
  END LOOP;
END;
$$;

ALTER FUNCTION trg_thread_verdict_deal_clock() RENAME TO trg_thread_verdict_clocks;
ALTER TRIGGER capture_thread_verdict_deal_clock ON capture_thread_verdict RENAME TO capture_thread_verdict_clocks;

CREATE OR REPLACE FUNCTION trg_thread_verdict_clocks() RETURNS trigger
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
    PERFORM refresh_clocks_of_thread(OLD.thread_key, OLD.user_id);
  END IF;
  IF is_personal THEN
    PERFORM refresh_clocks_of_thread(NEW.thread_key, NEW.user_id);
  END IF;
  RETURN NULL;
END;
$$;

DROP FUNCTION refresh_deal_clocks_of_thread(text, uuid);

-- A row's thread decides which seat's hold applies to it, so a change of
-- thread refreshes its clocks too.
DROP TRIGGER activity_last_activity ON activity;
CREATE TRIGGER activity_last_activity
	AFTER UPDATE OF occurred_at, archived_at, audience, audience_reason, thread_key, origin, kind, meeting_status, occurred_precision, call_outcome, bulk_mail_attested ON activity
	FOR EACH ROW
	WHEN (old.occurred_at IS DISTINCT FROM new.occurred_at
	   OR old.archived_at IS DISTINCT FROM new.archived_at
	   OR old.audience IS DISTINCT FROM new.audience
	   OR old.audience_reason IS DISTINCT FROM new.audience_reason
	   OR old.thread_key IS DISTINCT FROM new.thread_key
	   OR old.origin IS DISTINCT FROM new.origin
	   OR old.kind IS DISTINCT FROM new.kind
	   OR old.meeting_status IS DISTINCT FROM new.meeting_status
	   OR old.occurred_precision IS DISTINCT FROM new.occurred_precision
	   OR old.call_outcome IS DISTINCT FROM new.call_outcome
	   OR old.bulk_mail_attested IS DISTINCT FROM new.bulk_mail_attested)
	EXECUTE FUNCTION trg_activity_last_activity();

CREATE FUNCTION refresh_clocks_of_activity(aid uuid) RETURNS void
LANGUAGE plpgsql AS $$
DECLARE
  reached record;
BEGIN
  FOR reached IN
     SELECT reach.kind, reach.id FROM last_activity_reach(
         ARRAY(SELECT contact_id FROM activity_link WHERE activity_id = aid AND contact_id IS NOT NULL),
         ARRAY(SELECT deal_id FROM activity_link WHERE activity_id = aid AND deal_id IS NOT NULL),
         ARRAY(SELECT company_id FROM activity_link WHERE activity_id = aid AND company_id IS NOT NULL)) reach
      WHERE reach.kind IN ('company', 'deal')
      ORDER BY reach.kind, reach.id
  LOOP
    PERFORM move_last_activity(reached.kind::regclass, reached.id);
  END LOOP;
END;
$$;

-- A seat's import of a message on a thread it already holds as personal puts
-- the message under that hold, and removing the import takes it out again.
-- Either way the deal and company clocks the message reaches move.
CREATE FUNCTION trg_capture_import_clocks() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
  imported capture_import;
BEGIN
  IF TG_OP = 'DELETE' THEN
    imported := OLD;
  ELSE
    imported := NEW;
  END IF;
  IF imported.user_id IS NOT NULL AND EXISTS (
       SELECT 1 FROM activity a
         JOIN capture_thread_verdict hold ON hold.user_id = imported.user_id AND hold.thread_key = a.thread_key
        WHERE a.id = imported.activity_id AND a.thread_key <> ''
          AND hold.kind = 'personal' AND hold.status IN ('held', 'held_by_owner', 'unsure')) THEN
    PERFORM refresh_clocks_of_activity(imported.activity_id);
  END IF;
  RETURN NULL;
END;
$$;

CREATE TRIGGER capture_import_clocks
	AFTER INSERT OR DELETE ON capture_import
	FOR EACH ROW
	EXECUTE FUNCTION trg_capture_import_clocks();
