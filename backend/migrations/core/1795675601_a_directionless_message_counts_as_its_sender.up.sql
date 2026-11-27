-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- A message stored with no direction counts as its sender's when its one
-- `from` row is unambiguously one contact: no member's row, and no other live
-- contact holding its address. The strength fold now reads the row's user_id,
-- so the participant UPDATE trigger compares it too. The
-- directionless_sender_strength_dates data task refolds the contacts it moves.

SET LOCAL lock_timeout = '3s';

CREATE OR REPLACE FUNCTION contact_strength_dates(cid uuid)
RETURNS TABLE (interaction_at timestamptz, engaged_at timestamptz, inbound_at timestamptz, inbound_precision text,
               outbound_at timestamptz, outbound_precision text)
LANGUAGE sql STABLE AS $$
  SELECT max(a.occurred_at),
		       max(a.occurred_at) FILTER (WHERE (((a.direction = 'inbound' AND (EXISTS (
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
		         WHERE other.email = a.counterparty_email AND other.contact_id <> cid))))) OR (a.direction IS NULL AND a.kind IN ('email','message') AND EXISTS (
		SELECT 1 FROM activity_participant df
		 WHERE df.activity_id = a.id AND df.role = 'from' AND df.user_id IS NULL AND CASE WHEN df.address IS NULL
		              OR EXISTS (SELECT 1 FROM contact_email se
		                          WHERE se.contact_id = df.contact_id AND se.email = lower(df.address)
		                            AND (se.archived_at IS NULL OR EXISTS (
		                                 SELECT 1 FROM contact_email ne
		                                  WHERE ne.email = lower(df.address) AND ne.archived_at IS NULL
		                                    AND ne.contact_id <> df.contact_id)))
		            THEN df.contact_id = cid
		            ELSE (EXISTS (SELECT 1 FROM contact_email own
		         WHERE own.contact_id = cid AND own.archived_at IS NULL AND own.email = lower(df.address))
		 AND NOT EXISTS (SELECT 1 FROM contact_email other
		         WHERE other.email = lower(df.address) AND other.contact_id <> cid)) END
		   AND NOT EXISTS (SELECT 1 FROM contact_email held
		         WHERE held.email = lower(df.address) AND held.archived_at IS NULL AND held.contact_id <> cid)
		   AND NOT EXISTS (SELECT 1 FROM activity_participant df2
		         WHERE df2.activity_id = a.id AND df2.role = 'from' AND df2.id <> df.id)))) OR (a.kind IN ('meeting','call') AND (EXISTS (SELECT 1 FROM activity_participant tp
		 WHERE tp.activity_id = a.id AND tp.contact_id = cid AND tp.role IN ('from','to','cc','bcc','attendee','organizer')
		   AND tp.response IS DISTINCT FROM 'declined')
	   OR NOT EXISTS (SELECT 1 FROM activity_participant tp
		 WHERE tp.activity_id = a.id AND tp.contact_id IS NOT NULL))))),
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
		         WHERE other.email = a.counterparty_email AND other.contact_id <> cid))))) OR (a.direction IS NULL AND a.kind IN ('email','message') AND EXISTS (
		SELECT 1 FROM activity_participant df
		 WHERE df.activity_id = a.id AND df.role = 'from' AND df.user_id IS NULL AND CASE WHEN df.address IS NULL
		              OR EXISTS (SELECT 1 FROM contact_email se
		                          WHERE se.contact_id = df.contact_id AND se.email = lower(df.address)
		                            AND (se.archived_at IS NULL OR EXISTS (
		                                 SELECT 1 FROM contact_email ne
		                                  WHERE ne.email = lower(df.address) AND ne.archived_at IS NULL
		                                    AND ne.contact_id <> df.contact_id)))
		            THEN df.contact_id = cid
		            ELSE (EXISTS (SELECT 1 FROM contact_email own
		         WHERE own.contact_id = cid AND own.archived_at IS NULL AND own.email = lower(df.address))
		 AND NOT EXISTS (SELECT 1 FROM contact_email other
		         WHERE other.email = lower(df.address) AND other.contact_id <> cid)) END
		   AND NOT EXISTS (SELECT 1 FROM contact_email held
		         WHERE held.email = lower(df.address) AND held.archived_at IS NULL AND held.contact_id <> cid)
		   AND NOT EXISTS (SELECT 1 FROM activity_participant df2
		         WHERE df2.activity_id = a.id AND df2.role = 'from' AND df2.id <> df.id))))),
		       (CASE WHEN max(a.occurred_at) FILTER (WHERE a.occurred_precision = 'instant' AND (((a.direction = 'inbound' AND (EXISTS (
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
		         WHERE other.email = a.counterparty_email AND other.contact_id <> cid))))) OR (a.direction IS NULL AND a.kind IN ('email','message') AND EXISTS (
		SELECT 1 FROM activity_participant df
		 WHERE df.activity_id = a.id AND df.role = 'from' AND df.user_id IS NULL AND CASE WHEN df.address IS NULL
		              OR EXISTS (SELECT 1 FROM contact_email se
		                          WHERE se.contact_id = df.contact_id AND se.email = lower(df.address)
		                            AND (se.archived_at IS NULL OR EXISTS (
		                                 SELECT 1 FROM contact_email ne
		                                  WHERE ne.email = lower(df.address) AND ne.archived_at IS NULL
		                                    AND ne.contact_id <> df.contact_id)))
		            THEN df.contact_id = cid
		            ELSE (EXISTS (SELECT 1 FROM contact_email own
		         WHERE own.contact_id = cid AND own.archived_at IS NULL AND own.email = lower(df.address))
		 AND NOT EXISTS (SELECT 1 FROM contact_email other
		         WHERE other.email = lower(df.address) AND other.contact_id <> cid)) END
		   AND NOT EXISTS (SELECT 1 FROM contact_email held
		         WHERE held.email = lower(df.address) AND held.archived_at IS NULL AND held.contact_id <> cid)
		   AND NOT EXISTS (SELECT 1 FROM activity_participant df2
		         WHERE df2.activity_id = a.id AND df2.role = 'from' AND df2.id <> df.id)))))) = max(a.occurred_at) FILTER (WHERE ((a.direction = 'inbound' AND (EXISTS (
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
		         WHERE other.email = a.counterparty_email AND other.contact_id <> cid))))) OR (a.direction IS NULL AND a.kind IN ('email','message') AND EXISTS (
		SELECT 1 FROM activity_participant df
		 WHERE df.activity_id = a.id AND df.role = 'from' AND df.user_id IS NULL AND CASE WHEN df.address IS NULL
		              OR EXISTS (SELECT 1 FROM contact_email se
		                          WHERE se.contact_id = df.contact_id AND se.email = lower(df.address)
		                            AND (se.archived_at IS NULL OR EXISTS (
		                                 SELECT 1 FROM contact_email ne
		                                  WHERE ne.email = lower(df.address) AND ne.archived_at IS NULL
		                                    AND ne.contact_id <> df.contact_id)))
		            THEN df.contact_id = cid
		            ELSE (EXISTS (SELECT 1 FROM contact_email own
		         WHERE own.contact_id = cid AND own.archived_at IS NULL AND own.email = lower(df.address))
		 AND NOT EXISTS (SELECT 1 FROM contact_email other
		         WHERE other.email = lower(df.address) AND other.contact_id <> cid)) END
		   AND NOT EXISTS (SELECT 1 FROM contact_email held
		         WHERE held.email = lower(df.address) AND held.archived_at IS NULL AND held.contact_id <> cid)
		   AND NOT EXISTS (SELECT 1 FROM activity_participant df2
		         WHERE df2.activity_id = a.id AND df2.role = 'from' AND df2.id <> df.id))))) THEN 'instant'
		WHEN max(a.occurred_at) FILTER (WHERE ((a.direction = 'inbound' AND (EXISTS (
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
		         WHERE other.email = a.counterparty_email AND other.contact_id <> cid))))) OR (a.direction IS NULL AND a.kind IN ('email','message') AND EXISTS (
		SELECT 1 FROM activity_participant df
		 WHERE df.activity_id = a.id AND df.role = 'from' AND df.user_id IS NULL AND CASE WHEN df.address IS NULL
		              OR EXISTS (SELECT 1 FROM contact_email se
		                          WHERE se.contact_id = df.contact_id AND se.email = lower(df.address)
		                            AND (se.archived_at IS NULL OR EXISTS (
		                                 SELECT 1 FROM contact_email ne
		                                  WHERE ne.email = lower(df.address) AND ne.archived_at IS NULL
		                                    AND ne.contact_id <> df.contact_id)))
		            THEN df.contact_id = cid
		            ELSE (EXISTS (SELECT 1 FROM contact_email own
		         WHERE own.contact_id = cid AND own.archived_at IS NULL AND own.email = lower(df.address))
		 AND NOT EXISTS (SELECT 1 FROM contact_email other
		         WHERE other.email = lower(df.address) AND other.contact_id <> cid)) END
		   AND NOT EXISTS (SELECT 1 FROM contact_email held
		         WHERE held.email = lower(df.address) AND held.archived_at IS NULL AND held.contact_id <> cid)
		   AND NOT EXISTS (SELECT 1 FROM activity_participant df2
		         WHERE df2.activity_id = a.id AND df2.role = 'from' AND df2.id <> df.id))))) IS NOT NULL THEN 'day' END),
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

CREATE OR REPLACE FUNCTION contact_strength_dates_among(cid uuid, aids uuid[])
RETURNS TABLE (interaction_at timestamptz, engaged_at timestamptz, inbound_at timestamptz, inbound_precision text,
               outbound_at timestamptz, outbound_precision text)
LANGUAGE sql STABLE AS $$
  SELECT max(a.occurred_at),
		       max(a.occurred_at) FILTER (WHERE (((a.direction = 'inbound' AND (EXISTS (
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
		         WHERE other.email = a.counterparty_email AND other.contact_id <> cid))))) OR (a.direction IS NULL AND a.kind IN ('email','message') AND EXISTS (
		SELECT 1 FROM activity_participant df
		 WHERE df.activity_id = a.id AND df.role = 'from' AND df.user_id IS NULL AND CASE WHEN df.address IS NULL
		              OR EXISTS (SELECT 1 FROM contact_email se
		                          WHERE se.contact_id = df.contact_id AND se.email = lower(df.address)
		                            AND (se.archived_at IS NULL OR EXISTS (
		                                 SELECT 1 FROM contact_email ne
		                                  WHERE ne.email = lower(df.address) AND ne.archived_at IS NULL
		                                    AND ne.contact_id <> df.contact_id)))
		            THEN df.contact_id = cid
		            ELSE (EXISTS (SELECT 1 FROM contact_email own
		         WHERE own.contact_id = cid AND own.archived_at IS NULL AND own.email = lower(df.address))
		 AND NOT EXISTS (SELECT 1 FROM contact_email other
		         WHERE other.email = lower(df.address) AND other.contact_id <> cid)) END
		   AND NOT EXISTS (SELECT 1 FROM contact_email held
		         WHERE held.email = lower(df.address) AND held.archived_at IS NULL AND held.contact_id <> cid)
		   AND NOT EXISTS (SELECT 1 FROM activity_participant df2
		         WHERE df2.activity_id = a.id AND df2.role = 'from' AND df2.id <> df.id)))) OR (a.kind IN ('meeting','call') AND (EXISTS (SELECT 1 FROM activity_participant tp
		 WHERE tp.activity_id = a.id AND tp.contact_id = cid AND tp.role IN ('from','to','cc','bcc','attendee','organizer')
		   AND tp.response IS DISTINCT FROM 'declined')
	   OR NOT EXISTS (SELECT 1 FROM activity_participant tp
		 WHERE tp.activity_id = a.id AND tp.contact_id IS NOT NULL))))),
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
		         WHERE other.email = a.counterparty_email AND other.contact_id <> cid))))) OR (a.direction IS NULL AND a.kind IN ('email','message') AND EXISTS (
		SELECT 1 FROM activity_participant df
		 WHERE df.activity_id = a.id AND df.role = 'from' AND df.user_id IS NULL AND CASE WHEN df.address IS NULL
		              OR EXISTS (SELECT 1 FROM contact_email se
		                          WHERE se.contact_id = df.contact_id AND se.email = lower(df.address)
		                            AND (se.archived_at IS NULL OR EXISTS (
		                                 SELECT 1 FROM contact_email ne
		                                  WHERE ne.email = lower(df.address) AND ne.archived_at IS NULL
		                                    AND ne.contact_id <> df.contact_id)))
		            THEN df.contact_id = cid
		            ELSE (EXISTS (SELECT 1 FROM contact_email own
		         WHERE own.contact_id = cid AND own.archived_at IS NULL AND own.email = lower(df.address))
		 AND NOT EXISTS (SELECT 1 FROM contact_email other
		         WHERE other.email = lower(df.address) AND other.contact_id <> cid)) END
		   AND NOT EXISTS (SELECT 1 FROM contact_email held
		         WHERE held.email = lower(df.address) AND held.archived_at IS NULL AND held.contact_id <> cid)
		   AND NOT EXISTS (SELECT 1 FROM activity_participant df2
		         WHERE df2.activity_id = a.id AND df2.role = 'from' AND df2.id <> df.id))))),
		       (CASE WHEN max(a.occurred_at) FILTER (WHERE a.occurred_precision = 'instant' AND (((a.direction = 'inbound' AND (EXISTS (
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
		         WHERE other.email = a.counterparty_email AND other.contact_id <> cid))))) OR (a.direction IS NULL AND a.kind IN ('email','message') AND EXISTS (
		SELECT 1 FROM activity_participant df
		 WHERE df.activity_id = a.id AND df.role = 'from' AND df.user_id IS NULL AND CASE WHEN df.address IS NULL
		              OR EXISTS (SELECT 1 FROM contact_email se
		                          WHERE se.contact_id = df.contact_id AND se.email = lower(df.address)
		                            AND (se.archived_at IS NULL OR EXISTS (
		                                 SELECT 1 FROM contact_email ne
		                                  WHERE ne.email = lower(df.address) AND ne.archived_at IS NULL
		                                    AND ne.contact_id <> df.contact_id)))
		            THEN df.contact_id = cid
		            ELSE (EXISTS (SELECT 1 FROM contact_email own
		         WHERE own.contact_id = cid AND own.archived_at IS NULL AND own.email = lower(df.address))
		 AND NOT EXISTS (SELECT 1 FROM contact_email other
		         WHERE other.email = lower(df.address) AND other.contact_id <> cid)) END
		   AND NOT EXISTS (SELECT 1 FROM contact_email held
		         WHERE held.email = lower(df.address) AND held.archived_at IS NULL AND held.contact_id <> cid)
		   AND NOT EXISTS (SELECT 1 FROM activity_participant df2
		         WHERE df2.activity_id = a.id AND df2.role = 'from' AND df2.id <> df.id)))))) = max(a.occurred_at) FILTER (WHERE ((a.direction = 'inbound' AND (EXISTS (
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
		         WHERE other.email = a.counterparty_email AND other.contact_id <> cid))))) OR (a.direction IS NULL AND a.kind IN ('email','message') AND EXISTS (
		SELECT 1 FROM activity_participant df
		 WHERE df.activity_id = a.id AND df.role = 'from' AND df.user_id IS NULL AND CASE WHEN df.address IS NULL
		              OR EXISTS (SELECT 1 FROM contact_email se
		                          WHERE se.contact_id = df.contact_id AND se.email = lower(df.address)
		                            AND (se.archived_at IS NULL OR EXISTS (
		                                 SELECT 1 FROM contact_email ne
		                                  WHERE ne.email = lower(df.address) AND ne.archived_at IS NULL
		                                    AND ne.contact_id <> df.contact_id)))
		            THEN df.contact_id = cid
		            ELSE (EXISTS (SELECT 1 FROM contact_email own
		         WHERE own.contact_id = cid AND own.archived_at IS NULL AND own.email = lower(df.address))
		 AND NOT EXISTS (SELECT 1 FROM contact_email other
		         WHERE other.email = lower(df.address) AND other.contact_id <> cid)) END
		   AND NOT EXISTS (SELECT 1 FROM contact_email held
		         WHERE held.email = lower(df.address) AND held.archived_at IS NULL AND held.contact_id <> cid)
		   AND NOT EXISTS (SELECT 1 FROM activity_participant df2
		         WHERE df2.activity_id = a.id AND df2.role = 'from' AND df2.id <> df.id))))) THEN 'instant'
		WHEN max(a.occurred_at) FILTER (WHERE ((a.direction = 'inbound' AND (EXISTS (
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
		         WHERE other.email = a.counterparty_email AND other.contact_id <> cid))))) OR (a.direction IS NULL AND a.kind IN ('email','message') AND EXISTS (
		SELECT 1 FROM activity_participant df
		 WHERE df.activity_id = a.id AND df.role = 'from' AND df.user_id IS NULL AND CASE WHEN df.address IS NULL
		              OR EXISTS (SELECT 1 FROM contact_email se
		                          WHERE se.contact_id = df.contact_id AND se.email = lower(df.address)
		                            AND (se.archived_at IS NULL OR EXISTS (
		                                 SELECT 1 FROM contact_email ne
		                                  WHERE ne.email = lower(df.address) AND ne.archived_at IS NULL
		                                    AND ne.contact_id <> df.contact_id)))
		            THEN df.contact_id = cid
		            ELSE (EXISTS (SELECT 1 FROM contact_email own
		         WHERE own.contact_id = cid AND own.archived_at IS NULL AND own.email = lower(df.address))
		 AND NOT EXISTS (SELECT 1 FROM contact_email other
		         WHERE other.email = lower(df.address) AND other.contact_id <> cid)) END
		   AND NOT EXISTS (SELECT 1 FROM contact_email held
		         WHERE held.email = lower(df.address) AND held.archived_at IS NULL AND held.contact_id <> cid)
		   AND NOT EXISTS (SELECT 1 FROM activity_participant df2
		         WHERE df2.activity_id = a.id AND df2.role = 'from' AND df2.id <> df.id))))) IS NOT NULL THEN 'day' END),
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

CREATE OR REPLACE FUNCTION trg_activity_participant_strength_dates_moved() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  PERFORM touch_strength_dates_of_activities(ARRAY(
    SELECT DISTINCT unnest(ARRAY[o.activity_id, n.activity_id])
      FROM moved_from o JOIN moved_to n ON n.id = o.id
     WHERE (o.activity_id, o.role, o.contact_id, o.address, o.response, o.user_id)
           IS DISTINCT FROM (n.activity_id, n.role, n.contact_id, n.address, n.response, n.user_id)));
  RETURN NULL;
END;
$$;
