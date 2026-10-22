-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- last_activity_at on a deal, a contact and a company counts only what has
-- happened: an activity dated after now() no longer moves the clock. A booked
-- meeting next week left a deal looking touched today, and a future-dated email
-- did the same. The stalled flag and deal health read a booked meeting ahead on
-- their own (deals.MeetingAheadSQL), so they keep counting it.
--
-- The added bound is relstrength.LastActivityCountsSQL("a", "now()") verbatim,
-- in every arm, held there by backend/gates/lastactivitymeetingrule_test.go.
-- last_activity_reach keeps no bound: it must name every record whose clock a
-- write can move.
--
-- No row changes when a meeting's start passes, so the past_activity_clock_refold
-- job refolds the records an activity reached once its time has come. Its
-- watermark, activity_clock_refold_mark, starts empty, and a run with no mark
-- starts at the moment this migration was applied: the old rule already
-- counted every activity dated before it. Stored clocks the old rule left in
-- the future are moved back by the past_activity_clocks data task.
--
-- move_last_activity now writes only a value that changed, so a refold over
-- records it does not move rewrites nothing.
SET LOCAL lock_timeout = '3s';

CREATE OR REPLACE FUNCTION last_activity_of_deal(did uuid) RETURNS timestamptz
    LANGUAGE sql STABLE
    AS $$
  SELECT max(a.occurred_at)
    FROM activity_link l
    JOIN activity a ON a.id = l.activity_id AND a.archived_at IS NULL
     AND a.audience = 'workspace'
     AND a.origin NOT IN ('system_remediation', 'system_notice')
     AND ((a.kind <> 'meeting' OR (a.meeting_status IS NULL OR a.meeting_status IN ('booked','held'))) AND a.occurred_at <= now())
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
     AND ((a.kind <> 'meeting' OR (a.meeting_status IS NULL OR a.meeting_status IN ('booked','held'))) AND a.occurred_at <= now())
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
       AND ((a.kind <> 'meeting' OR (a.meeting_status IS NULL OR a.meeting_status IN ('booked','held'))) AND a.occurred_at <= now())
     WHERE l.company_id = cid
    UNION ALL
    -- Filed against one of its deals.
    SELECT max(a.occurred_at)
      FROM deal d
      JOIN activity_link l ON l.deal_id = d.id
      JOIN activity a ON a.id = l.activity_id AND a.archived_at IS NULL
       AND a.audience = 'workspace'
       AND a.origin NOT IN ('system_remediation', 'system_notice')
       AND ((a.kind <> 'meeting' OR (a.meeting_status IS NULL OR a.meeting_status IN ('booked','held'))) AND a.occurred_at <= now())
     WHERE d.company_id = cid
    UNION ALL
    -- Filed against a contact it currently employs, sent while they worked there.
    SELECT max(a.occurred_at)
      FROM relationship r
      JOIN activity_link l ON l.contact_id = r.contact_id
      JOIN activity a ON a.id = l.activity_id AND a.archived_at IS NULL
       AND a.audience = 'workspace'
       AND a.origin NOT IN ('system_remediation', 'system_notice')
       AND ((a.kind <> 'meeting' OR (a.meeting_status IS NULL OR a.meeting_status IN ('booked','held'))) AND a.occurred_at <= now())
     WHERE r.company_id = cid AND r.kind = 'employment'
       AND r.ended_at IS NULL AND r.archived_at IS NULL
       AND coalesce(r.started_at::timestamp AT TIME ZONE 'UTC', r.first_observed_at, a.occurred_at) <= a.occurred_at
  ) arms
$$;

CREATE OR REPLACE FUNCTION move_last_activity(tbl regclass, rid uuid) RETURNS void
    LANGUAGE plpgsql
    AS $$
DECLARE
  v timestamptz;
BEGIN
  IF rid IS NULL THEN RETURN; END IF;
  CASE tbl
    WHEN 'contact'::regclass THEN
      PERFORM 1 FROM contact WHERE id = rid FOR UPDATE;
      v := last_activity_of_contact(rid);
      PERFORM set_config('margince.last_activity_move', 'on', true);
      UPDATE contact SET last_activity_at = v WHERE id = rid AND last_activity_at IS DISTINCT FROM v;
    WHEN 'deal'::regclass THEN
      PERFORM 1 FROM deal WHERE id = rid FOR UPDATE;
      v := last_activity_of_deal(rid);
      PERFORM set_config('margince.last_activity_move', 'on', true);
      UPDATE deal SET last_activity_at = v WHERE id = rid AND last_activity_at IS DISTINCT FROM v;
    WHEN 'company'::regclass THEN
      PERFORM 1 FROM company WHERE id = rid FOR UPDATE;
      v := last_activity_of_company(rid);
      PERFORM set_config('margince.last_activity_move', 'on', true);
      UPDATE company SET last_activity_at = v WHERE id = rid AND last_activity_at IS DISTINCT FROM v;
  END CASE;
  PERFORM set_config('margince.last_activity_move', 'off', true);
END;
$$;

CREATE TABLE activity_clock_refold_mark (
    singleton boolean DEFAULT true NOT NULL,
    passed_up_to timestamptz NOT NULL,
    passed_id uuid NOT NULL,
    CONSTRAINT activity_clock_refold_mark_pkey PRIMARY KEY (singleton),
    CONSTRAINT activity_clock_refold_mark_singleton_check CHECK (singleton)
);
