-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- A contact stores the dates its relationship strength reads from its whole
-- history: last interaction, last engaged, and the last message each way with
-- its precision. The company page and the list reads then fold only the
-- 90-day window. The columns hold what contact_strength_dates gives at now().
-- Its body is the recency read contacts.StoredStrengthDatesSQL renders, held
-- there by backend/gates/storedstrengthdates_test.go.
--
-- Statement triggers keep the dates in the writer's transaction, one UPDATE of
-- contact per statement. A change that can only add a newer date merges the
-- dates of the activities it touched. A change to an activity a stored date
-- came from recomputes the contact from its history, so an archive, an
-- audience restriction, an erasure or an anonymization lowers what it gave.
-- An archived contact stores no dates. The clock refold recomputes a contact
-- once an activity's time has come, and the contact_strength_dates data task
-- folds the live contacts this migration finds.
--
-- Lock order: each function takes contact rows after the statement that fired
-- it has written its own rows, in contact id order, the order the last-activity
-- writers take.
SET LOCAL lock_timeout = '3s';

ALTER TABLE contact
    ADD COLUMN IF NOT EXISTS strength_last_interaction_at timestamptz,
    ADD COLUMN IF NOT EXISTS strength_last_engaged_at timestamptz,
    ADD COLUMN IF NOT EXISTS strength_last_inbound_at timestamptz,
    ADD COLUMN IF NOT EXISTS strength_last_inbound_day boolean,
    ADD COLUMN IF NOT EXISTS strength_last_outbound_at timestamptz,
    ADD COLUMN IF NOT EXISTS strength_last_outbound_day boolean,
    ADD COLUMN IF NOT EXISTS strength_folded boolean NOT NULL DEFAULT false;

ALTER TABLE contact ALTER COLUMN strength_folded SET DEFAULT true;

-- The clock refold finds the contacts still to fold through this index, so a
-- pass reads only those, not the whole table.
CREATE INDEX IF NOT EXISTS idx_contact_strength_unfolded ON contact (id)
    WHERE NOT strength_folded AND archived_at IS NULL;

COMMENT ON COLUMN contact.strength_last_inbound_day IS
    'Whether strength_last_inbound_at names a day entry rather than an instant. NULL when the date is NULL.';
COMMENT ON COLUMN contact.strength_last_outbound_day IS
    'Whether strength_last_outbound_at names a day entry rather than an instant. NULL when the date is NULL.';
COMMENT ON COLUMN contact.strength_folded IS
    'Whether the strength_last_* columns were folded from the contact''s history. False until the backfill reaches the contact.';

CREATE FUNCTION contact_strength_dates(cid uuid)
RETURNS TABLE (interaction_at timestamptz, engaged_at timestamptz, inbound_at timestamptz, inbound_precision text,
               outbound_at timestamptz, outbound_precision text)
LANGUAGE sql STABLE AS $$
  SELECT max(a.occurred_at),
		       max(a.occurred_at) FILTER (WHERE ((a.direction = 'inbound' AND (EXISTS (
		SELECT 1 FROM activity_participant ap
		 WHERE ap.activity_id = a.id AND ap.role = 'from'
		   AND CASE WHEN ap.address IS NULL
		              OR EXISTS (SELECT 1 FROM contact_email se
		                          WHERE se.contact_id = ap.contact_id AND se.email = lower(ap.address)
		                            AND (se.archived_at IS NULL OR EXISTS (
		                                 SELECT 1 FROM contact_email ne
		                                  WHERE ne.email = lower(ap.address) AND ne.archived_at IS NULL
		                                    AND ne.contact_id <> ap.contact_id)))
		            THEN ap.contact_id = cid
		            ELSE (EXISTS (SELECT 1 FROM contact_email own
		         WHERE own.contact_id = cid AND own.archived_at IS NULL AND own.email = lower(ap.address))
		 AND NOT EXISTS (SELECT 1 FROM contact_email other
		         WHERE other.email = lower(ap.address) AND other.contact_id <> cid)) END) OR (NOT EXISTS (SELECT 1 FROM activity_participant af
		 WHERE af.activity_id = a.id AND af.role = 'from')
		AND (EXISTS (SELECT 1 FROM contact_email own
		         WHERE own.contact_id = cid AND own.archived_at IS NULL AND own.email = a.counterparty_email)
		 AND NOT EXISTS (SELECT 1 FROM contact_email other
		         WHERE other.email = a.counterparty_email AND other.contact_id <> cid))))) OR (a.kind IN ('meeting','call') AND (EXISTS (SELECT 1 FROM activity_participant tp
		 WHERE tp.activity_id = a.id AND tp.contact_id = cid AND tp.role IN ('from','to','cc','bcc','attendee','organizer')
		   AND tp.response IS DISTINCT FROM 'declined')
	   OR NOT EXISTS (SELECT 1 FROM activity_participant tp
		 WHERE tp.activity_id = a.id AND tp.contact_id IS NOT NULL))))),
		       max(a.occurred_at) FILTER (WHERE (a.direction = 'inbound' AND (EXISTS (
		SELECT 1 FROM activity_participant ap
		 WHERE ap.activity_id = a.id AND ap.role = 'from'
		   AND CASE WHEN ap.address IS NULL
		              OR EXISTS (SELECT 1 FROM contact_email se
		                          WHERE se.contact_id = ap.contact_id AND se.email = lower(ap.address)
		                            AND (se.archived_at IS NULL OR EXISTS (
		                                 SELECT 1 FROM contact_email ne
		                                  WHERE ne.email = lower(ap.address) AND ne.archived_at IS NULL
		                                    AND ne.contact_id <> ap.contact_id)))
		            THEN ap.contact_id = cid
		            ELSE (EXISTS (SELECT 1 FROM contact_email own
		         WHERE own.contact_id = cid AND own.archived_at IS NULL AND own.email = lower(ap.address))
		 AND NOT EXISTS (SELECT 1 FROM contact_email other
		         WHERE other.email = lower(ap.address) AND other.contact_id <> cid)) END) OR (NOT EXISTS (SELECT 1 FROM activity_participant af
		 WHERE af.activity_id = a.id AND af.role = 'from')
		AND (EXISTS (SELECT 1 FROM contact_email own
		         WHERE own.contact_id = cid AND own.archived_at IS NULL AND own.email = a.counterparty_email)
		 AND NOT EXISTS (SELECT 1 FROM contact_email other
		         WHERE other.email = a.counterparty_email AND other.contact_id <> cid)))))),
		       (CASE WHEN max(a.occurred_at) FILTER (WHERE a.occurred_precision = 'instant' AND ((a.direction = 'inbound' AND (EXISTS (
		SELECT 1 FROM activity_participant ap
		 WHERE ap.activity_id = a.id AND ap.role = 'from'
		   AND CASE WHEN ap.address IS NULL
		              OR EXISTS (SELECT 1 FROM contact_email se
		                          WHERE se.contact_id = ap.contact_id AND se.email = lower(ap.address)
		                            AND (se.archived_at IS NULL OR EXISTS (
		                                 SELECT 1 FROM contact_email ne
		                                  WHERE ne.email = lower(ap.address) AND ne.archived_at IS NULL
		                                    AND ne.contact_id <> ap.contact_id)))
		            THEN ap.contact_id = cid
		            ELSE (EXISTS (SELECT 1 FROM contact_email own
		         WHERE own.contact_id = cid AND own.archived_at IS NULL AND own.email = lower(ap.address))
		 AND NOT EXISTS (SELECT 1 FROM contact_email other
		         WHERE other.email = lower(ap.address) AND other.contact_id <> cid)) END) OR (NOT EXISTS (SELECT 1 FROM activity_participant af
		 WHERE af.activity_id = a.id AND af.role = 'from')
		AND (EXISTS (SELECT 1 FROM contact_email own
		         WHERE own.contact_id = cid AND own.archived_at IS NULL AND own.email = a.counterparty_email)
		 AND NOT EXISTS (SELECT 1 FROM contact_email other
		         WHERE other.email = a.counterparty_email AND other.contact_id <> cid))))))) = max(a.occurred_at) FILTER (WHERE (a.direction = 'inbound' AND (EXISTS (
		SELECT 1 FROM activity_participant ap
		 WHERE ap.activity_id = a.id AND ap.role = 'from'
		   AND CASE WHEN ap.address IS NULL
		              OR EXISTS (SELECT 1 FROM contact_email se
		                          WHERE se.contact_id = ap.contact_id AND se.email = lower(ap.address)
		                            AND (se.archived_at IS NULL OR EXISTS (
		                                 SELECT 1 FROM contact_email ne
		                                  WHERE ne.email = lower(ap.address) AND ne.archived_at IS NULL
		                                    AND ne.contact_id <> ap.contact_id)))
		            THEN ap.contact_id = cid
		            ELSE (EXISTS (SELECT 1 FROM contact_email own
		         WHERE own.contact_id = cid AND own.archived_at IS NULL AND own.email = lower(ap.address))
		 AND NOT EXISTS (SELECT 1 FROM contact_email other
		         WHERE other.email = lower(ap.address) AND other.contact_id <> cid)) END) OR (NOT EXISTS (SELECT 1 FROM activity_participant af
		 WHERE af.activity_id = a.id AND af.role = 'from')
		AND (EXISTS (SELECT 1 FROM contact_email own
		         WHERE own.contact_id = cid AND own.archived_at IS NULL AND own.email = a.counterparty_email)
		 AND NOT EXISTS (SELECT 1 FROM contact_email other
		         WHERE other.email = a.counterparty_email AND other.contact_id <> cid)))))) THEN 'instant'
		WHEN max(a.occurred_at) FILTER (WHERE (a.direction = 'inbound' AND (EXISTS (
		SELECT 1 FROM activity_participant ap
		 WHERE ap.activity_id = a.id AND ap.role = 'from'
		   AND CASE WHEN ap.address IS NULL
		              OR EXISTS (SELECT 1 FROM contact_email se
		                          WHERE se.contact_id = ap.contact_id AND se.email = lower(ap.address)
		                            AND (se.archived_at IS NULL OR EXISTS (
		                                 SELECT 1 FROM contact_email ne
		                                  WHERE ne.email = lower(ap.address) AND ne.archived_at IS NULL
		                                    AND ne.contact_id <> ap.contact_id)))
		            THEN ap.contact_id = cid
		            ELSE (EXISTS (SELECT 1 FROM contact_email own
		         WHERE own.contact_id = cid AND own.archived_at IS NULL AND own.email = lower(ap.address))
		 AND NOT EXISTS (SELECT 1 FROM contact_email other
		         WHERE other.email = lower(ap.address) AND other.contact_id <> cid)) END) OR (NOT EXISTS (SELECT 1 FROM activity_participant af
		 WHERE af.activity_id = a.id AND af.role = 'from')
		AND (EXISTS (SELECT 1 FROM contact_email own
		         WHERE own.contact_id = cid AND own.archived_at IS NULL AND own.email = a.counterparty_email)
		 AND NOT EXISTS (SELECT 1 FROM contact_email other
		         WHERE other.email = a.counterparty_email AND other.contact_id <> cid)))))) IS NOT NULL THEN 'day' END),
		       max(a.occurred_at) FILTER (WHERE (a.direction = 'outbound')),
		       (CASE WHEN max(a.occurred_at) FILTER (WHERE a.occurred_precision = 'day' AND ((a.direction = 'outbound'))) = max(a.occurred_at) FILTER (WHERE (a.direction = 'outbound')) THEN 'day'
		WHEN max(a.occurred_at) FILTER (WHERE (a.direction = 'outbound')) IS NOT NULL THEN 'instant' END)
    FROM activity a
    JOIN activity_link l ON l.activity_id = a.id AND l.contact_id = cid
   WHERE (a.internal_mail_authorization IS NULL AND a.kind IN ('email','call','meeting','message') AND ((a.kind <> 'meeting' OR ((a.meeting_status IS NULL OR a.meeting_status IN ('booked','held')) AND (a.occurred_at <= greatest(now(), ((date_trunc('day', CAST(now() AS timestamptz) AT TIME ZONE COALESCE((SELECT st.value #>> '{}' FROM setting st WHERE st.key = 'installation.timezone'), 'UTC')) + interval '12 hours') AT TIME ZONE 'UTC')) AND (a.occurred_precision = 'day' AND a.occurred_at < ((date_trunc('day', CAST(now() AS timestamptz) AT TIME ZONE COALESCE((SELECT st.value #>> '{}' FROM setting st WHERE st.key = 'installation.timezone'), 'UTC')) + interval '12 hours') AT TIME ZONE 'UTC') OR a.occurred_precision = 'instant' AND a.occurred_at <= now())))) AND (a.kind <> 'call' OR (a.call_outcome IS NULL OR a.call_outcome IN ('reached','put_through')))) AND (a.origin NOT IN ('system_remediation','system_notice') AND NOT (a.kind = 'email' AND a.bulk_mail_attested) AND NOT a.forwarded_to_self AND (a.kind <> 'email' OR (NOT a.sender_is_machine AND a.calendar_method IS NULL))) AND ((a.kind = 'meeting' AND (a.occurred_at <= greatest(now(), ((date_trunc('day', CAST(now() AS timestamptz) AT TIME ZONE COALESCE((SELECT st.value #>> '{}' FROM setting st WHERE st.key = 'installation.timezone'), 'UTC')) + interval '12 hours') AT TIME ZONE 'UTC')) AND (a.occurred_precision = 'day' AND a.occurred_at < ((date_trunc('day', CAST(now() AS timestamptz) AT TIME ZONE COALESCE((SELECT st.value #>> '{}' FROM setting st WHERE st.key = 'installation.timezone'), 'UTC')) + interval '12 hours') AT TIME ZONE 'UTC') OR a.occurred_precision = 'instant' AND a.occurred_at <= now()))) OR (a.kind <> 'meeting' AND (a.occurred_at <= greatest(now(), ((date_trunc('day', CAST(now() AS timestamptz) AT TIME ZONE COALESCE((SELECT st.value #>> '{}' FROM setting st WHERE st.key = 'installation.timezone'), 'UTC')) + interval '12 hours') AT TIME ZONE 'UTC')) AND (a.occurred_precision = 'day' OR a.occurred_at <= now()))))) AND a.archived_at IS NULL AND (a.audience = 'workspace' AND
 (a.internal_mail_authorization IS NOT NULL OR NOT EXISTS (
 SELECT 1 FROM capture_import thread_hold_import
 JOIN capture_thread_verdict thread_hold ON thread_hold.user_id=thread_hold_import.user_id
 AND thread_hold.thread_key=a.thread_key
 WHERE thread_hold_import.activity_id=a.id AND thread_hold.status IN ('held', 'held_by_owner', 'unsure')))) AND (a.kind <> 'task' OR a.source_activity_id IS NULL AND coalesce(a.source_system, '') <> 'lead_replied' OR EXISTS (SELECT 1 FROM activity follow_up_source WHERE follow_up_source.id = CASE WHEN a.source_system = 'lead_replied' THEN coalesce(a.source_activity_id, CASE WHEN a.source_id ~* '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$' THEN a.source_id::uuid END) ELSE a.source_activity_id END AND (follow_up_source.audience = 'workspace' AND
 (follow_up_source.internal_mail_authorization IS NOT NULL OR NOT EXISTS (
 SELECT 1 FROM capture_import thread_hold_import
 JOIN capture_thread_verdict thread_hold ON thread_hold.user_id=thread_hold_import.user_id
 AND thread_hold.thread_key=follow_up_source.thread_key
 WHERE thread_hold_import.activity_id=follow_up_source.id AND thread_hold.status IN ('held', 'held_by_owner', 'unsure')))) AND follow_up_source.restricted_at IS NULL))
$$;

CREATE FUNCTION contact_strength_dates_among(cid uuid, aids uuid[])
RETURNS TABLE (interaction_at timestamptz, engaged_at timestamptz, inbound_at timestamptz, inbound_precision text,
               outbound_at timestamptz, outbound_precision text)
LANGUAGE sql STABLE AS $$
  SELECT max(a.occurred_at),
		       max(a.occurred_at) FILTER (WHERE ((a.direction = 'inbound' AND (EXISTS (
		SELECT 1 FROM activity_participant ap
		 WHERE ap.activity_id = a.id AND ap.role = 'from'
		   AND CASE WHEN ap.address IS NULL
		              OR EXISTS (SELECT 1 FROM contact_email se
		                          WHERE se.contact_id = ap.contact_id AND se.email = lower(ap.address)
		                            AND (se.archived_at IS NULL OR EXISTS (
		                                 SELECT 1 FROM contact_email ne
		                                  WHERE ne.email = lower(ap.address) AND ne.archived_at IS NULL
		                                    AND ne.contact_id <> ap.contact_id)))
		            THEN ap.contact_id = cid
		            ELSE (EXISTS (SELECT 1 FROM contact_email own
		         WHERE own.contact_id = cid AND own.archived_at IS NULL AND own.email = lower(ap.address))
		 AND NOT EXISTS (SELECT 1 FROM contact_email other
		         WHERE other.email = lower(ap.address) AND other.contact_id <> cid)) END) OR (NOT EXISTS (SELECT 1 FROM activity_participant af
		 WHERE af.activity_id = a.id AND af.role = 'from')
		AND (EXISTS (SELECT 1 FROM contact_email own
		         WHERE own.contact_id = cid AND own.archived_at IS NULL AND own.email = a.counterparty_email)
		 AND NOT EXISTS (SELECT 1 FROM contact_email other
		         WHERE other.email = a.counterparty_email AND other.contact_id <> cid))))) OR (a.kind IN ('meeting','call') AND (EXISTS (SELECT 1 FROM activity_participant tp
		 WHERE tp.activity_id = a.id AND tp.contact_id = cid AND tp.role IN ('from','to','cc','bcc','attendee','organizer')
		   AND tp.response IS DISTINCT FROM 'declined')
	   OR NOT EXISTS (SELECT 1 FROM activity_participant tp
		 WHERE tp.activity_id = a.id AND tp.contact_id IS NOT NULL))))),
		       max(a.occurred_at) FILTER (WHERE (a.direction = 'inbound' AND (EXISTS (
		SELECT 1 FROM activity_participant ap
		 WHERE ap.activity_id = a.id AND ap.role = 'from'
		   AND CASE WHEN ap.address IS NULL
		              OR EXISTS (SELECT 1 FROM contact_email se
		                          WHERE se.contact_id = ap.contact_id AND se.email = lower(ap.address)
		                            AND (se.archived_at IS NULL OR EXISTS (
		                                 SELECT 1 FROM contact_email ne
		                                  WHERE ne.email = lower(ap.address) AND ne.archived_at IS NULL
		                                    AND ne.contact_id <> ap.contact_id)))
		            THEN ap.contact_id = cid
		            ELSE (EXISTS (SELECT 1 FROM contact_email own
		         WHERE own.contact_id = cid AND own.archived_at IS NULL AND own.email = lower(ap.address))
		 AND NOT EXISTS (SELECT 1 FROM contact_email other
		         WHERE other.email = lower(ap.address) AND other.contact_id <> cid)) END) OR (NOT EXISTS (SELECT 1 FROM activity_participant af
		 WHERE af.activity_id = a.id AND af.role = 'from')
		AND (EXISTS (SELECT 1 FROM contact_email own
		         WHERE own.contact_id = cid AND own.archived_at IS NULL AND own.email = a.counterparty_email)
		 AND NOT EXISTS (SELECT 1 FROM contact_email other
		         WHERE other.email = a.counterparty_email AND other.contact_id <> cid)))))),
		       (CASE WHEN max(a.occurred_at) FILTER (WHERE a.occurred_precision = 'instant' AND ((a.direction = 'inbound' AND (EXISTS (
		SELECT 1 FROM activity_participant ap
		 WHERE ap.activity_id = a.id AND ap.role = 'from'
		   AND CASE WHEN ap.address IS NULL
		              OR EXISTS (SELECT 1 FROM contact_email se
		                          WHERE se.contact_id = ap.contact_id AND se.email = lower(ap.address)
		                            AND (se.archived_at IS NULL OR EXISTS (
		                                 SELECT 1 FROM contact_email ne
		                                  WHERE ne.email = lower(ap.address) AND ne.archived_at IS NULL
		                                    AND ne.contact_id <> ap.contact_id)))
		            THEN ap.contact_id = cid
		            ELSE (EXISTS (SELECT 1 FROM contact_email own
		         WHERE own.contact_id = cid AND own.archived_at IS NULL AND own.email = lower(ap.address))
		 AND NOT EXISTS (SELECT 1 FROM contact_email other
		         WHERE other.email = lower(ap.address) AND other.contact_id <> cid)) END) OR (NOT EXISTS (SELECT 1 FROM activity_participant af
		 WHERE af.activity_id = a.id AND af.role = 'from')
		AND (EXISTS (SELECT 1 FROM contact_email own
		         WHERE own.contact_id = cid AND own.archived_at IS NULL AND own.email = a.counterparty_email)
		 AND NOT EXISTS (SELECT 1 FROM contact_email other
		         WHERE other.email = a.counterparty_email AND other.contact_id <> cid))))))) = max(a.occurred_at) FILTER (WHERE (a.direction = 'inbound' AND (EXISTS (
		SELECT 1 FROM activity_participant ap
		 WHERE ap.activity_id = a.id AND ap.role = 'from'
		   AND CASE WHEN ap.address IS NULL
		              OR EXISTS (SELECT 1 FROM contact_email se
		                          WHERE se.contact_id = ap.contact_id AND se.email = lower(ap.address)
		                            AND (se.archived_at IS NULL OR EXISTS (
		                                 SELECT 1 FROM contact_email ne
		                                  WHERE ne.email = lower(ap.address) AND ne.archived_at IS NULL
		                                    AND ne.contact_id <> ap.contact_id)))
		            THEN ap.contact_id = cid
		            ELSE (EXISTS (SELECT 1 FROM contact_email own
		         WHERE own.contact_id = cid AND own.archived_at IS NULL AND own.email = lower(ap.address))
		 AND NOT EXISTS (SELECT 1 FROM contact_email other
		         WHERE other.email = lower(ap.address) AND other.contact_id <> cid)) END) OR (NOT EXISTS (SELECT 1 FROM activity_participant af
		 WHERE af.activity_id = a.id AND af.role = 'from')
		AND (EXISTS (SELECT 1 FROM contact_email own
		         WHERE own.contact_id = cid AND own.archived_at IS NULL AND own.email = a.counterparty_email)
		 AND NOT EXISTS (SELECT 1 FROM contact_email other
		         WHERE other.email = a.counterparty_email AND other.contact_id <> cid)))))) THEN 'instant'
		WHEN max(a.occurred_at) FILTER (WHERE (a.direction = 'inbound' AND (EXISTS (
		SELECT 1 FROM activity_participant ap
		 WHERE ap.activity_id = a.id AND ap.role = 'from'
		   AND CASE WHEN ap.address IS NULL
		              OR EXISTS (SELECT 1 FROM contact_email se
		                          WHERE se.contact_id = ap.contact_id AND se.email = lower(ap.address)
		                            AND (se.archived_at IS NULL OR EXISTS (
		                                 SELECT 1 FROM contact_email ne
		                                  WHERE ne.email = lower(ap.address) AND ne.archived_at IS NULL
		                                    AND ne.contact_id <> ap.contact_id)))
		            THEN ap.contact_id = cid
		            ELSE (EXISTS (SELECT 1 FROM contact_email own
		         WHERE own.contact_id = cid AND own.archived_at IS NULL AND own.email = lower(ap.address))
		 AND NOT EXISTS (SELECT 1 FROM contact_email other
		         WHERE other.email = lower(ap.address) AND other.contact_id <> cid)) END) OR (NOT EXISTS (SELECT 1 FROM activity_participant af
		 WHERE af.activity_id = a.id AND af.role = 'from')
		AND (EXISTS (SELECT 1 FROM contact_email own
		         WHERE own.contact_id = cid AND own.archived_at IS NULL AND own.email = a.counterparty_email)
		 AND NOT EXISTS (SELECT 1 FROM contact_email other
		         WHERE other.email = a.counterparty_email AND other.contact_id <> cid)))))) IS NOT NULL THEN 'day' END),
		       max(a.occurred_at) FILTER (WHERE (a.direction = 'outbound')),
		       (CASE WHEN max(a.occurred_at) FILTER (WHERE a.occurred_precision = 'day' AND ((a.direction = 'outbound'))) = max(a.occurred_at) FILTER (WHERE (a.direction = 'outbound')) THEN 'day'
		WHEN max(a.occurred_at) FILTER (WHERE (a.direction = 'outbound')) IS NOT NULL THEN 'instant' END)
    FROM activity a
    JOIN activity_link l ON l.activity_id = a.id AND l.contact_id = cid
   WHERE (a.internal_mail_authorization IS NULL AND a.kind IN ('email','call','meeting','message') AND ((a.kind <> 'meeting' OR ((a.meeting_status IS NULL OR a.meeting_status IN ('booked','held')) AND (a.occurred_at <= greatest(now(), ((date_trunc('day', CAST(now() AS timestamptz) AT TIME ZONE COALESCE((SELECT st.value #>> '{}' FROM setting st WHERE st.key = 'installation.timezone'), 'UTC')) + interval '12 hours') AT TIME ZONE 'UTC')) AND (a.occurred_precision = 'day' AND a.occurred_at < ((date_trunc('day', CAST(now() AS timestamptz) AT TIME ZONE COALESCE((SELECT st.value #>> '{}' FROM setting st WHERE st.key = 'installation.timezone'), 'UTC')) + interval '12 hours') AT TIME ZONE 'UTC') OR a.occurred_precision = 'instant' AND a.occurred_at <= now())))) AND (a.kind <> 'call' OR (a.call_outcome IS NULL OR a.call_outcome IN ('reached','put_through')))) AND (a.origin NOT IN ('system_remediation','system_notice') AND NOT (a.kind = 'email' AND a.bulk_mail_attested) AND NOT a.forwarded_to_self AND (a.kind <> 'email' OR (NOT a.sender_is_machine AND a.calendar_method IS NULL))) AND ((a.kind = 'meeting' AND (a.occurred_at <= greatest(now(), ((date_trunc('day', CAST(now() AS timestamptz) AT TIME ZONE COALESCE((SELECT st.value #>> '{}' FROM setting st WHERE st.key = 'installation.timezone'), 'UTC')) + interval '12 hours') AT TIME ZONE 'UTC')) AND (a.occurred_precision = 'day' AND a.occurred_at < ((date_trunc('day', CAST(now() AS timestamptz) AT TIME ZONE COALESCE((SELECT st.value #>> '{}' FROM setting st WHERE st.key = 'installation.timezone'), 'UTC')) + interval '12 hours') AT TIME ZONE 'UTC') OR a.occurred_precision = 'instant' AND a.occurred_at <= now()))) OR (a.kind <> 'meeting' AND (a.occurred_at <= greatest(now(), ((date_trunc('day', CAST(now() AS timestamptz) AT TIME ZONE COALESCE((SELECT st.value #>> '{}' FROM setting st WHERE st.key = 'installation.timezone'), 'UTC')) + interval '12 hours') AT TIME ZONE 'UTC')) AND (a.occurred_precision = 'day' OR a.occurred_at <= now()))))) AND a.archived_at IS NULL AND (a.audience = 'workspace' AND
 (a.internal_mail_authorization IS NOT NULL OR NOT EXISTS (
 SELECT 1 FROM capture_import thread_hold_import
 JOIN capture_thread_verdict thread_hold ON thread_hold.user_id=thread_hold_import.user_id
 AND thread_hold.thread_key=a.thread_key
 WHERE thread_hold_import.activity_id=a.id AND thread_hold.status IN ('held', 'held_by_owner', 'unsure')))) AND (a.kind <> 'task' OR a.source_activity_id IS NULL AND coalesce(a.source_system, '') <> 'lead_replied' OR EXISTS (SELECT 1 FROM activity follow_up_source WHERE follow_up_source.id = CASE WHEN a.source_system = 'lead_replied' THEN coalesce(a.source_activity_id, CASE WHEN a.source_id ~* '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$' THEN a.source_id::uuid END) ELSE a.source_activity_id END AND (follow_up_source.audience = 'workspace' AND
 (follow_up_source.internal_mail_authorization IS NOT NULL OR NOT EXISTS (
 SELECT 1 FROM capture_import thread_hold_import
 JOIN capture_thread_verdict thread_hold ON thread_hold.user_id=thread_hold_import.user_id
 AND thread_hold.thread_key=follow_up_source.thread_key
 WHERE thread_hold_import.activity_id=follow_up_source.id AND thread_hold.status IN ('held', 'held_by_owner', 'unsure')))) AND follow_up_source.restricted_at IS NULL)) AND a.id = ANY (aids)
$$;

-- Writes each contact's dates from its whole history and marks it folded. An
-- archived contact gets none. It sets margince.last_activity_move, so the
-- version does not move.
CREATE FUNCTION refresh_contact_strength_dates(cids uuid[]) RETURNS void
LANGUAGE plpgsql AS $$
DECLARE
  moving text := current_setting('margince.last_activity_move', true);
BEGIN
  IF cids IS NULL OR cardinality(cids) = 0 THEN RETURN; END IF;
  PERFORM 1 FROM contact WHERE id = ANY (cids) ORDER BY id FOR UPDATE;
  PERFORM set_config('margince.last_activity_move', 'on', true);
  UPDATE contact p
     SET strength_last_interaction_at = f.interaction_at,
         strength_last_engaged_at = f.engaged_at,
         strength_last_inbound_at = f.inbound_at,
         strength_last_inbound_day = f.inbound_precision = 'day',
         strength_last_outbound_at = f.outbound_at,
         strength_last_outbound_day = f.outbound_precision = 'day',
         strength_folded = true
    FROM contact c
    LEFT JOIN LATERAL (SELECT * FROM contact_strength_dates(c.id) WHERE c.archived_at IS NULL) f ON true
   WHERE c.id = ANY (cids) AND p.id = c.id
     AND (p.strength_last_interaction_at, p.strength_last_engaged_at, p.strength_last_inbound_at,
          p.strength_last_inbound_day, p.strength_last_outbound_at, p.strength_last_outbound_day, p.strength_folded)
         IS DISTINCT FROM (f.interaction_at, f.engaged_at, f.inbound_at, f.inbound_precision = 'day',
          f.outbound_at, f.outbound_precision = 'day', true);
  PERFORM set_config('margince.last_activity_move', coalesce(moving, ''), true);
END;
$$;

-- Marks every live contact's dates unfolded, so readers fold the history until
-- they are folded again. A move of the installation zone runs it, because
-- whether a day entry has happened depends on the zone's today.
CREATE FUNCTION unfold_contact_strength_dates() RETURNS void
LANGUAGE plpgsql AS $$
DECLARE
  moving text := current_setting('margince.last_activity_move', true);
BEGIN
  PERFORM set_config('margince.last_activity_move', 'on', true);
  UPDATE contact SET strength_folded = false WHERE strength_folded AND archived_at IS NULL;
  PERFORM set_config('margince.last_activity_move', coalesce(moving, ''), true);
END;
$$;

-- Moves the dates of each contact after a statement changed the activities
-- paired with it. was is the date the activity held before the change:
-- -infinity when it held none, NULL when nobody knows. A contact is recomputed
-- from its history when a touched activity may have given a stored date it no
-- longer gives, or ties one at another precision. Otherwise the touched
-- activities can only add newer dates, and their own fold is merged in.
CREATE FUNCTION touch_contact_strength_dates(cids uuid[], aids uuid[], was timestamptz[]) RETURNS void
LANGUAGE plpgsql AS $$
DECLARE
  moving text := current_setting('margince.last_activity_move', true);
BEGIN
  IF cids IS NULL OR cardinality(cids) = 0 THEN RETURN; END IF;
  PERFORM 1 FROM contact WHERE id IN (SELECT unnest(cids)) ORDER BY id FOR UPDATE;
  PERFORM set_config('margince.last_activity_move', 'on', true);
  WITH grouped AS (
    SELECT t.cid, array_agg(t.aid) AS aids, array_agg(t.was) AS was, bool_or(t.was IS NULL) AS unknown
      FROM unnest(cids, aids, was) t(cid, aid, was)
     WHERE t.cid IS NOT NULL
     GROUP BY t.cid
  ), judged AS (
    SELECT p.id, f.*,
           g.unknown
           OR (p.strength_last_interaction_at = ANY (g.was) AND coalesce(f.interaction_at < p.strength_last_interaction_at, true))
           OR (p.strength_last_engaged_at = ANY (g.was) AND coalesce(f.engaged_at < p.strength_last_engaged_at, true))
           OR (p.strength_last_inbound_at = ANY (g.was) AND coalesce(f.inbound_at < p.strength_last_inbound_at, true))
           OR (p.strength_last_outbound_at = ANY (g.was) AND coalesce(f.outbound_at < p.strength_last_outbound_at, true))
           OR (f.inbound_at = p.strength_last_inbound_at
               AND (f.inbound_precision = 'day') IS DISTINCT FROM p.strength_last_inbound_day)
           OR (f.outbound_at = p.strength_last_outbound_at
               AND (f.outbound_precision = 'day') IS DISTINCT FROM p.strength_last_outbound_day) AS refold
      FROM grouped g
      JOIN contact p ON p.id = g.cid
     CROSS JOIN LATERAL contact_strength_dates_among(p.id, g.aids) f
     WHERE p.strength_folded AND p.archived_at IS NULL
  ), moved AS (
    SELECT j.id,
           CASE WHEN j.refold THEN h.interaction_at ELSE greatest(p.strength_last_interaction_at, j.interaction_at) END AS interaction_at,
           CASE WHEN j.refold THEN h.engaged_at ELSE greatest(p.strength_last_engaged_at, j.engaged_at) END AS engaged_at,
           CASE WHEN j.refold THEN h.inbound_at ELSE greatest(p.strength_last_inbound_at, j.inbound_at) END AS inbound_at,
           CASE WHEN j.refold THEN h.inbound_precision = 'day'
                WHEN j.inbound_at > coalesce(p.strength_last_inbound_at, '-infinity') THEN j.inbound_precision = 'day'
                ELSE p.strength_last_inbound_day END AS inbound_day,
           CASE WHEN j.refold THEN h.outbound_at ELSE greatest(p.strength_last_outbound_at, j.outbound_at) END AS outbound_at,
           CASE WHEN j.refold THEN h.outbound_precision = 'day'
                WHEN j.outbound_at > coalesce(p.strength_last_outbound_at, '-infinity') THEN j.outbound_precision = 'day'
                ELSE p.strength_last_outbound_day END AS outbound_day
      FROM judged j
      JOIN contact p ON p.id = j.id
      LEFT JOIN LATERAL (SELECT * FROM contact_strength_dates(j.id) WHERE j.refold) h ON true
  )
  UPDATE contact p
     SET strength_last_interaction_at = m.interaction_at,
         strength_last_engaged_at = m.engaged_at,
         strength_last_inbound_at = m.inbound_at,
         strength_last_inbound_day = m.inbound_day,
         strength_last_outbound_at = m.outbound_at,
         strength_last_outbound_day = m.outbound_day
    FROM moved m
   WHERE p.id = m.id
     AND (p.strength_last_interaction_at, p.strength_last_engaged_at, p.strength_last_inbound_at,
          p.strength_last_inbound_day, p.strength_last_outbound_at, p.strength_last_outbound_day)
         IS DISTINCT FROM (m.interaction_at, m.engaged_at, m.inbound_at, m.inbound_day, m.outbound_at, m.outbound_day);
  PERFORM set_config('margince.last_activity_move', coalesce(moving, ''), true);
END;
$$;

-- Touches every contact filed under the activities, each activity at the date
-- it holds now: a change beside an activity moves no date of its own.
CREATE FUNCTION touch_strength_dates_of_activities(aids uuid[]) RETURNS void
LANGUAGE plpgsql AS $$
DECLARE
  c uuid[];
  a uuid[];
  w timestamptz[];
BEGIN
  SELECT array_agg(l.contact_id), array_agg(l.activity_id), array_agg(x.occurred_at)
    INTO c, a, w
    FROM activity_link l
    LEFT JOIN activity x ON x.id = l.activity_id
   WHERE l.activity_id = ANY (aids) AND l.contact_id IS NOT NULL;
  PERFORM touch_contact_strength_dates(c, a, w);
END;
$$;

-- A seat's import and its verdict on a thread decide together whether the
-- verdict holds the activity, so a change to a verdict touches the activities
-- that seat imported on that thread. holds carries each (thread, seat) pair as
-- one value, so no two lists are paired by position.
CREATE FUNCTION touch_strength_dates_of_holds(holds jsonb) RETURNS void
LANGUAGE plpgsql AS $$
BEGIN
  PERFORM touch_strength_dates_of_activities(ARRAY(
    SELECT DISTINCT i.activity_id
      FROM jsonb_to_recordset(holds) h(thread_key text, user_id uuid)
      JOIN activity a ON a.thread_key = h.thread_key
      JOIN capture_import i ON i.activity_id = a.id AND i.user_id = h.user_id));
END;
$$;

CREATE FUNCTION trg_activity_link_strength_dates_added() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
  c uuid[];
  a uuid[];
  w timestamptz[];
BEGIN
  SELECT array_agg(n.contact_id), array_agg(n.activity_id), array_agg('-infinity'::timestamptz)
    INTO c, a, w FROM added n WHERE n.contact_id IS NOT NULL;
  PERFORM touch_contact_strength_dates(c, a, w);
  RETURN NULL;
END;
$$;

CREATE FUNCTION trg_activity_link_strength_dates_removed() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
  c uuid[];
  a uuid[];
  w timestamptz[];
BEGIN
  SELECT array_agg(o.contact_id), array_agg(o.activity_id), array_agg(x.occurred_at)
    INTO c, a, w FROM removed o LEFT JOIN activity x ON x.id = o.activity_id WHERE o.contact_id IS NOT NULL;
  PERFORM touch_contact_strength_dates(c, a, w);
  RETURN NULL;
END;
$$;

CREATE FUNCTION trg_activity_link_strength_dates_moved() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
  c uuid[];
  a uuid[];
  w timestamptz[];
BEGIN
  SELECT array_agg(p.contact_id), array_agg(p.activity_id), array_agg(p.was)
    INTO c, a, w
    FROM (SELECT o.contact_id, o.activity_id, x.occurred_at AS was
            FROM moved_from o JOIN moved_to n ON n.id = o.id
            LEFT JOIN activity x ON x.id = o.activity_id
           WHERE (o.contact_id, o.activity_id) IS DISTINCT FROM (n.contact_id, n.activity_id)
          UNION ALL
          SELECT n.contact_id, n.activity_id, '-infinity'
            FROM moved_from o JOIN moved_to n ON n.id = o.id
           WHERE (o.contact_id, o.activity_id) IS DISTINCT FROM (n.contact_id, n.activity_id)) p
   WHERE p.contact_id IS NOT NULL;
  PERFORM touch_contact_strength_dates(c, a, w);
  RETURN NULL;
END;
$$;

CREATE FUNCTION trg_activity_strength_dates() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
  c uuid[];
  a uuid[];
  w timestamptz[];
BEGIN
  SELECT array_agg(l.contact_id), array_agg(l.activity_id), array_agg(o.occurred_at)
    INTO c, a, w
    FROM changed_from o
    JOIN changed_to n ON n.id = o.id
    JOIN activity_link l ON l.activity_id = n.id AND l.contact_id IS NOT NULL
   WHERE (o.archived_at, o.audience, o.bulk_mail_attested, o.calendar_method, o.call_outcome, o.counterparty_email, o.direction, o.forwarded_to_self, o.internal_mail_authorization, o.kind, o.meeting_status, o.occurred_at, o.occurred_precision, o.origin, o.restricted_at, o.sender_is_machine, o.source_activity_id, o.source_id, o.source_system, o.thread_key)
         IS DISTINCT FROM (n.archived_at, n.audience, n.bulk_mail_attested, n.calendar_method, n.call_outcome, n.counterparty_email, n.direction, n.forwarded_to_self, n.internal_mail_authorization, n.kind, n.meeting_status, n.occurred_at, n.occurred_precision, n.origin, n.restricted_at, n.sender_is_machine, n.source_activity_id, n.source_id, n.source_system, n.thread_key);
  PERFORM touch_contact_strength_dates(c, a, w);
  RETURN NULL;
END;
$$;

CREATE FUNCTION trg_activity_participant_strength_dates_added() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  PERFORM touch_strength_dates_of_activities(ARRAY(SELECT DISTINCT n.activity_id FROM added n));
  RETURN NULL;
END;
$$;

CREATE FUNCTION trg_activity_participant_strength_dates_removed() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  PERFORM touch_strength_dates_of_activities(ARRAY(SELECT DISTINCT o.activity_id FROM removed o));
  RETURN NULL;
END;
$$;

CREATE FUNCTION trg_activity_participant_strength_dates_moved() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  PERFORM touch_strength_dates_of_activities(ARRAY(
    SELECT DISTINCT unnest(ARRAY[o.activity_id, n.activity_id])
      FROM moved_from o JOIN moved_to n ON n.id = o.id
     WHERE (o.activity_id, o.role, o.contact_id, o.address, o.response)
           IS DISTINCT FROM (n.activity_id, n.role, n.contact_id, n.address, n.response)));
  RETURN NULL;
END;
$$;

-- Whether an address names a contact depends on every contact that holds or
-- held it, so each of them is recomputed, and each contact a participant row
-- carrying the address stores. One with no filed activity has nothing to fold:
-- its dates are already empty, because the link that left took them along.
CREATE FUNCTION refresh_strength_dates_of_addresses(emails text[], holders uuid[]) RETURNS void
LANGUAGE plpgsql AS $$
BEGIN
  PERFORM refresh_contact_strength_dates(ARRAY(
    SELECT named.id FROM (
      SELECT h.id FROM unnest(holders) h(id)
      UNION SELECT ce.contact_id FROM contact_email ce WHERE ce.email = ANY (emails)
      UNION SELECT ap.contact_id FROM activity_participant ap
             WHERE lower(ap.address) = ANY (emails) AND ap.address IS NOT NULL AND ap.contact_id IS NOT NULL) named
     WHERE named.id IS NOT NULL
       AND EXISTS (SELECT 1 FROM activity_link l WHERE l.contact_id = named.id)));
END;
$$;

CREATE FUNCTION trg_contact_email_strength_dates_added() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  PERFORM refresh_strength_dates_of_addresses(ARRAY(SELECT n.email FROM added n), ARRAY(SELECT n.contact_id FROM added n));
  RETURN NULL;
END;
$$;

CREATE FUNCTION trg_contact_email_strength_dates_removed() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  PERFORM refresh_strength_dates_of_addresses(ARRAY(SELECT o.email FROM removed o), ARRAY(SELECT o.contact_id FROM removed o));
  RETURN NULL;
END;
$$;

CREATE FUNCTION trg_contact_email_strength_dates_moved() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  PERFORM refresh_strength_dates_of_addresses(
    ARRAY(SELECT unnest(ARRAY[o.email, n.email]) FROM moved_from o JOIN moved_to n ON n.id = o.id
           WHERE (o.contact_id, o.email, o.archived_at) IS DISTINCT FROM (n.contact_id, n.email, n.archived_at)),
    ARRAY(SELECT unnest(ARRAY[o.contact_id, n.contact_id]) FROM moved_from o JOIN moved_to n ON n.id = o.id
           WHERE (o.contact_id, o.email, o.archived_at) IS DISTINCT FROM (n.contact_id, n.email, n.archived_at)));
  RETURN NULL;
END;
$$;

CREATE FUNCTION trg_capture_import_strength_dates_added() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  PERFORM touch_strength_dates_of_activities(ARRAY(
    SELECT DISTINCT n.activity_id FROM added n JOIN activity a ON a.id = n.activity_id
      JOIN capture_thread_verdict v ON v.thread_key = a.thread_key AND v.user_id = n.user_id));
  RETURN NULL;
END;
$$;

CREATE FUNCTION trg_capture_import_strength_dates_removed() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  PERFORM touch_strength_dates_of_activities(ARRAY(
    SELECT DISTINCT o.activity_id FROM removed o JOIN activity a ON a.id = o.activity_id
      JOIN capture_thread_verdict v ON v.thread_key = a.thread_key AND v.user_id = o.user_id));
  RETURN NULL;
END;
$$;

CREATE FUNCTION trg_capture_import_strength_dates_moved() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  PERFORM touch_strength_dates_of_activities(ARRAY(
    SELECT DISTINCT unnest(ARRAY[o.activity_id, n.activity_id])
      FROM moved_from o JOIN moved_to n ON n.id = o.id
     WHERE (o.activity_id, o.user_id) IS DISTINCT FROM (n.activity_id, n.user_id)));
  RETURN NULL;
END;
$$;

CREATE FUNCTION trg_capture_thread_verdict_strength_dates_added() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  PERFORM touch_strength_dates_of_holds(
    (SELECT jsonb_agg(jsonb_build_object('thread_key', n.thread_key, 'user_id', n.user_id)) FROM added n));
  RETURN NULL;
END;
$$;

CREATE FUNCTION trg_capture_thread_verdict_strength_dates_removed() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  PERFORM touch_strength_dates_of_holds(
    (SELECT jsonb_agg(jsonb_build_object('thread_key', o.thread_key, 'user_id', o.user_id)) FROM removed o));
  RETURN NULL;
END;
$$;

CREATE FUNCTION trg_capture_thread_verdict_strength_dates_moved() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  PERFORM touch_strength_dates_of_holds((
    SELECT jsonb_agg(jsonb_build_object('thread_key', side.thread_key, 'user_id', side.user_id))
      FROM moved_from o
      JOIN moved_to n ON n.id = o.id
     CROSS JOIN LATERAL (VALUES (o.thread_key, o.user_id), (n.thread_key, n.user_id)) side(thread_key, user_id)
     WHERE (o.thread_key, o.user_id, o.status) IS DISTINCT FROM (n.thread_key, n.user_id, n.status)));
  RETURN NULL;
END;
$$;

-- none behind. One brought back is folded again from its history.
CREATE FUNCTION trg_contact_strength_dates_archive() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
  f record;
BEGIN
  IF NEW.archived_at IS NOT NULL THEN
    NEW.strength_last_interaction_at := NULL;
    NEW.strength_last_engaged_at := NULL;
    NEW.strength_last_inbound_at := NULL;
    NEW.strength_last_inbound_day := NULL;
    NEW.strength_last_outbound_at := NULL;
    NEW.strength_last_outbound_day := NULL;
    RETURN NEW;
  END IF;
  SELECT * INTO f FROM contact_strength_dates(NEW.id);
  NEW.strength_last_interaction_at := f.interaction_at;
  NEW.strength_last_engaged_at := f.engaged_at;
  NEW.strength_last_inbound_at := f.inbound_at;
  NEW.strength_last_inbound_day := f.inbound_precision = 'day';
  NEW.strength_last_outbound_at := f.outbound_at;
  NEW.strength_last_outbound_day := f.outbound_precision = 'day';
  NEW.strength_folded := true;
  RETURN NEW;
END;
$$;

CREATE TRIGGER activity_link_strength_dates_added
	AFTER INSERT ON activity_link REFERENCING NEW TABLE AS added
	FOR EACH STATEMENT EXECUTE FUNCTION trg_activity_link_strength_dates_added();

CREATE TRIGGER activity_link_strength_dates_removed
	AFTER DELETE ON activity_link REFERENCING OLD TABLE AS removed
	FOR EACH STATEMENT EXECUTE FUNCTION trg_activity_link_strength_dates_removed();

CREATE TRIGGER activity_link_strength_dates_moved
	AFTER UPDATE ON activity_link REFERENCING OLD TABLE AS moved_from NEW TABLE AS moved_to
	FOR EACH STATEMENT EXECUTE FUNCTION trg_activity_link_strength_dates_moved();

CREATE TRIGGER activity_participant_strength_dates_added
	AFTER INSERT ON activity_participant REFERENCING NEW TABLE AS added
	FOR EACH STATEMENT EXECUTE FUNCTION trg_activity_participant_strength_dates_added();

CREATE TRIGGER activity_participant_strength_dates_removed
	AFTER DELETE ON activity_participant REFERENCING OLD TABLE AS removed
	FOR EACH STATEMENT EXECUTE FUNCTION trg_activity_participant_strength_dates_removed();

CREATE TRIGGER activity_participant_strength_dates_moved
	AFTER UPDATE ON activity_participant REFERENCING OLD TABLE AS moved_from NEW TABLE AS moved_to
	FOR EACH STATEMENT EXECUTE FUNCTION trg_activity_participant_strength_dates_moved();

CREATE TRIGGER contact_email_strength_dates_added
	AFTER INSERT ON contact_email REFERENCING NEW TABLE AS added
	FOR EACH STATEMENT EXECUTE FUNCTION trg_contact_email_strength_dates_added();

CREATE TRIGGER contact_email_strength_dates_removed
	AFTER DELETE ON contact_email REFERENCING OLD TABLE AS removed
	FOR EACH STATEMENT EXECUTE FUNCTION trg_contact_email_strength_dates_removed();

CREATE TRIGGER contact_email_strength_dates_moved
	AFTER UPDATE ON contact_email REFERENCING OLD TABLE AS moved_from NEW TABLE AS moved_to
	FOR EACH STATEMENT EXECUTE FUNCTION trg_contact_email_strength_dates_moved();

CREATE TRIGGER capture_import_strength_dates_added
	AFTER INSERT ON capture_import REFERENCING NEW TABLE AS added
	FOR EACH STATEMENT EXECUTE FUNCTION trg_capture_import_strength_dates_added();

CREATE TRIGGER capture_import_strength_dates_removed
	AFTER DELETE ON capture_import REFERENCING OLD TABLE AS removed
	FOR EACH STATEMENT EXECUTE FUNCTION trg_capture_import_strength_dates_removed();

CREATE TRIGGER capture_import_strength_dates_moved
	AFTER UPDATE ON capture_import REFERENCING OLD TABLE AS moved_from NEW TABLE AS moved_to
	FOR EACH STATEMENT EXECUTE FUNCTION trg_capture_import_strength_dates_moved();

CREATE TRIGGER capture_thread_verdict_strength_dates_added
	AFTER INSERT ON capture_thread_verdict REFERENCING NEW TABLE AS added
	FOR EACH STATEMENT EXECUTE FUNCTION trg_capture_thread_verdict_strength_dates_added();

CREATE TRIGGER capture_thread_verdict_strength_dates_removed
	AFTER DELETE ON capture_thread_verdict REFERENCING OLD TABLE AS removed
	FOR EACH STATEMENT EXECUTE FUNCTION trg_capture_thread_verdict_strength_dates_removed();

CREATE TRIGGER capture_thread_verdict_strength_dates_moved
	AFTER UPDATE ON capture_thread_verdict REFERENCING OLD TABLE AS moved_from NEW TABLE AS moved_to
	FOR EACH STATEMENT EXECUTE FUNCTION trg_capture_thread_verdict_strength_dates_moved();

CREATE TRIGGER activity_strength_dates
	AFTER UPDATE ON activity REFERENCING OLD TABLE AS changed_from NEW TABLE AS changed_to
	FOR EACH STATEMENT EXECUTE FUNCTION trg_activity_strength_dates();

CREATE TRIGGER contact_strength_dates_archive
	BEFORE UPDATE OF archived_at ON contact
	FOR EACH ROW WHEN (old.archived_at IS DISTINCT FROM new.archived_at)
	EXECUTE FUNCTION trg_contact_strength_dates_archive();
