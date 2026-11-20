-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- A deal link says whether its activity is about the deal. deal_relevance is
-- relevant, unrelated or uncertain, and relevance_source says whether a human
-- or the classifier judged it. NULL is a link nobody judged, and it counts as
-- before, so no row changes. Only a deal link carries a judgement.
--
-- The deal clock now reads relstrength.DealEvidenceSQL with the deal: a link
-- judged unrelated or uncertain moves no deal clock. A new link's advance reads
-- that one link, so a link inserted already judged moves nothing. Changing the
-- judgement updates the link row, whose trigger refreshes the deal clock. The
-- checks are added NOT VALID; the next migration validates them.
SET LOCAL lock_timeout = '3s';

ALTER TABLE activity_link
    ADD COLUMN IF NOT EXISTS deal_relevance text,
    ADD COLUMN IF NOT EXISTS relevance_source text,
    ADD CONSTRAINT activity_link_deal_relevance_check
        CHECK (deal_relevance = ANY (ARRAY['relevant', 'unrelated', 'uncertain'])) NOT VALID,
    ADD CONSTRAINT activity_link_relevance_source_check
        CHECK (relevance_source = ANY (ARRAY['human', 'classifier'])) NOT VALID,
    ADD CONSTRAINT activity_link_deal_relevance_shape
        CHECK ((deal_relevance IS NULL) = (relevance_source IS NULL)
               AND (deal_relevance IS NULL OR entity_type = 'deal')) NOT VALID;

COMMENT ON COLUMN activity_link.deal_relevance IS
    'Whether the activity is about the linked deal: relevant, unrelated or uncertain. NULL means nobody judged it, and it counts.';
COMMENT ON COLUMN activity_link.relevance_source IS
    'Who judged deal_relevance: human or classifier. The classifier never overwrites a human judgement.';

DROP FUNCTION last_activity_of_deal_activity(uuid);

CREATE FUNCTION last_activity_of_deal_activity(aid uuid, did uuid) RETURNS timestamptz
LANGUAGE sql STABLE AS $$SELECT max(a.occurred_at)
    FROM activity_link l
    JOIN activity a ON a.id = l.activity_id AND a.archived_at IS NULL
     AND a.audience IN ('workspace', 'participants')
     AND ((coalesce(a.audience_reason, '') <> 'personal' AND NOT EXISTS (
 SELECT 1 FROM capture_import thread_hold_import
 JOIN capture_thread_verdict thread_hold ON thread_hold.user_id=thread_hold_import.user_id
 AND thread_hold.thread_key=a.thread_key
 WHERE thread_hold_import.activity_id=a.id AND thread_hold.kind = 'personal' AND thread_hold.status IN ('held', 'held_by_owner', 'unsure'))) AND NOT EXISTS (SELECT 1 FROM activity_link judged WHERE judged.activity_id = a.id AND judged.deal_id = l.deal_id AND judged.deal_relevance IN ('unrelated', 'uncertain')))
     AND a.origin NOT IN ('system_remediation', 'system_notice')
     AND (a.kind IN ('email','message','meeting','call','note') AND ((a.kind <> 'meeting' OR ((a.meeting_status IS NULL OR a.meeting_status IN ('booked','held')) AND (a.occurred_at <= greatest(now(), ((date_trunc('day', CAST(now() AS timestamptz) AT TIME ZONE COALESCE((SELECT st.value #>> '{}' FROM setting st WHERE st.key = 'installation.timezone'), 'UTC')) + interval '12 hours') AT TIME ZONE 'UTC')) AND (a.occurred_precision = 'day' AND a.occurred_at < ((date_trunc('day', CAST(now() AS timestamptz) AT TIME ZONE COALESCE((SELECT st.value #>> '{}' FROM setting st WHERE st.key = 'installation.timezone'), 'UTC')) + interval '12 hours') AT TIME ZONE 'UTC') OR a.occurred_precision = 'instant' AND a.occurred_at <= now())))) AND (a.kind <> 'call' OR (a.call_outcome IS NULL OR a.call_outcome IN ('reached','put_through')))) AND NOT (a.kind = 'note' AND a.origin = 'agent') AND NOT (a.kind = 'email' AND a.bulk_mail_attested) AND ((a.kind = 'meeting' AND (a.occurred_at <= greatest(now(), ((date_trunc('day', CAST(now() AS timestamptz) AT TIME ZONE COALESCE((SELECT st.value #>> '{}' FROM setting st WHERE st.key = 'installation.timezone'), 'UTC')) + interval '12 hours') AT TIME ZONE 'UTC')) AND (a.occurred_precision = 'day' AND a.occurred_at < ((date_trunc('day', CAST(now() AS timestamptz) AT TIME ZONE COALESCE((SELECT st.value #>> '{}' FROM setting st WHERE st.key = 'installation.timezone'), 'UTC')) + interval '12 hours') AT TIME ZONE 'UTC') OR a.occurred_precision = 'instant' AND a.occurred_at <= now()))) OR (a.kind <> 'meeting' AND (a.occurred_at <= greatest(now(), ((date_trunc('day', CAST(now() AS timestamptz) AT TIME ZONE COALESCE((SELECT st.value #>> '{}' FROM setting st WHERE st.key = 'installation.timezone'), 'UTC')) + interval '12 hours') AT TIME ZONE 'UTC')) AND (a.occurred_precision = 'day' OR a.occurred_at <= now())))))
   WHERE l.activity_id = aid AND l.deal_id = did
$$;

CREATE OR REPLACE FUNCTION last_activity_of_deal(did uuid) RETURNS timestamptz
    LANGUAGE sql STABLE
    AS $$
  SELECT max(a.occurred_at)
    FROM activity_link l
    JOIN activity a ON a.id = l.activity_id AND a.archived_at IS NULL
     AND a.audience IN ('workspace', 'participants')
     AND ((coalesce(a.audience_reason, '') <> 'personal' AND NOT EXISTS (
 SELECT 1 FROM capture_import thread_hold_import
 JOIN capture_thread_verdict thread_hold ON thread_hold.user_id=thread_hold_import.user_id
 AND thread_hold.thread_key=a.thread_key
 WHERE thread_hold_import.activity_id=a.id AND thread_hold.kind = 'personal' AND thread_hold.status IN ('held', 'held_by_owner', 'unsure'))) AND NOT EXISTS (SELECT 1 FROM activity_link judged WHERE judged.activity_id = a.id AND judged.deal_id = l.deal_id AND judged.deal_relevance IN ('unrelated', 'uncertain')))
     AND a.origin NOT IN ('system_remediation', 'system_notice')
     AND (a.kind IN ('email','message','meeting','call','note') AND ((a.kind <> 'meeting' OR ((a.meeting_status IS NULL OR a.meeting_status IN ('booked','held')) AND (a.occurred_at <= greatest(now(), ((date_trunc('day', CAST(now() AS timestamptz) AT TIME ZONE COALESCE((SELECT st.value #>> '{}' FROM setting st WHERE st.key = 'installation.timezone'), 'UTC')) + interval '12 hours') AT TIME ZONE 'UTC')) AND (a.occurred_precision = 'day' AND a.occurred_at < ((date_trunc('day', CAST(now() AS timestamptz) AT TIME ZONE COALESCE((SELECT st.value #>> '{}' FROM setting st WHERE st.key = 'installation.timezone'), 'UTC')) + interval '12 hours') AT TIME ZONE 'UTC') OR a.occurred_precision = 'instant' AND a.occurred_at <= now())))) AND (a.kind <> 'call' OR (a.call_outcome IS NULL OR a.call_outcome IN ('reached','put_through')))) AND NOT (a.kind = 'note' AND a.origin = 'agent') AND NOT (a.kind = 'email' AND a.bulk_mail_attested) AND ((a.kind = 'meeting' AND (a.occurred_at <= greatest(now(), ((date_trunc('day', CAST(now() AS timestamptz) AT TIME ZONE COALESCE((SELECT st.value #>> '{}' FROM setting st WHERE st.key = 'installation.timezone'), 'UTC')) + interval '12 hours') AT TIME ZONE 'UTC')) AND (a.occurred_precision = 'day' AND a.occurred_at < ((date_trunc('day', CAST(now() AS timestamptz) AT TIME ZONE COALESCE((SELECT st.value #>> '{}' FROM setting st WHERE st.key = 'installation.timezone'), 'UTC')) + interval '12 hours') AT TIME ZONE 'UTC') OR a.occurred_precision = 'instant' AND a.occurred_at <= now()))) OR (a.kind <> 'meeting' AND (a.occurred_at <= greatest(now(), ((date_trunc('day', CAST(now() AS timestamptz) AT TIME ZONE COALESCE((SELECT st.value #>> '{}' FROM setting st WHERE st.key = 'installation.timezone'), 'UTC')) + interval '12 hours') AT TIME ZONE 'UTC')) AND (a.occurred_precision = 'day' OR a.occurred_at <= now())))))
   WHERE l.deal_id = did
$$;

CREATE OR REPLACE FUNCTION trg_activity_link_last_activity() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
  at timestamptz;
  deal_at timestamptz;
  reached record;
BEGIN
  IF TG_OP = 'INSERT' THEN
    IF NEW.contact_id IS NULL AND NEW.deal_id IS NULL AND NEW.company_id IS NULL THEN
      RETURN NULL;
    END IF;
    at := last_activity_of_activity(NEW.activity_id);
    IF NEW.deal_id IS NOT NULL THEN
      deal_at := last_activity_of_deal_activity(NEW.activity_id, NEW.deal_id);
    END IF;
    IF at IS NOT NULL OR deal_at IS NOT NULL THEN
      FOR reached IN
         SELECT reach.kind, reach.id
           FROM last_activity_reach(ARRAY[NEW.contact_id], ARRAY[NEW.deal_id], ARRAY[NEW.company_id]) reach
          ORDER BY reach.kind, reach.id
      LOOP
        PERFORM advance_last_activity(reached.kind::regclass, reached.id,
          CASE reached.kind WHEN 'company' THEN last_activity_of_company_activity(NEW.activity_id, reached.id)
                            WHEN 'deal' THEN deal_at ELSE at END);
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
