-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Reverse of the up migration: the clocks count an activity whenever it is
-- dated, and move_last_activity rewrites a record even when its value stands.
-- Stored clocks refold under the restored rule as each record's activities
-- next change.
SET LOCAL lock_timeout = '3s';

DROP TABLE activity_clock_refold_mark;

CREATE OR REPLACE FUNCTION last_activity_of_deal(did uuid) RETURNS timestamptz
    LANGUAGE sql STABLE
    AS $$
  SELECT max(a.occurred_at)
    FROM activity_link l
    JOIN activity a ON a.id = l.activity_id AND a.archived_at IS NULL
     AND a.audience = 'workspace'
     AND a.origin NOT IN ('system_remediation', 'system_notice')
     AND (a.kind <> 'meeting' OR (a.meeting_status IS NULL OR a.meeting_status IN ('booked','held')))
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
     AND (a.kind <> 'meeting' OR (a.meeting_status IS NULL OR a.meeting_status IN ('booked','held')))
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
       AND (a.kind <> 'meeting' OR (a.meeting_status IS NULL OR a.meeting_status IN ('booked','held')))
     WHERE l.company_id = cid
    UNION ALL
    -- Filed against one of its deals.
    SELECT max(a.occurred_at)
      FROM deal d
      JOIN activity_link l ON l.deal_id = d.id
      JOIN activity a ON a.id = l.activity_id AND a.archived_at IS NULL
       AND a.audience = 'workspace'
       AND a.origin NOT IN ('system_remediation', 'system_notice')
       AND (a.kind <> 'meeting' OR (a.meeting_status IS NULL OR a.meeting_status IN ('booked','held')))
     WHERE d.company_id = cid
    UNION ALL
    -- Filed against a contact it currently employs, sent while they worked there.
    SELECT max(a.occurred_at)
      FROM relationship r
      JOIN activity_link l ON l.contact_id = r.contact_id
      JOIN activity a ON a.id = l.activity_id AND a.archived_at IS NULL
       AND a.audience = 'workspace'
       AND a.origin NOT IN ('system_remediation', 'system_notice')
       AND (a.kind <> 'meeting' OR (a.meeting_status IS NULL OR a.meeting_status IN ('booked','held')))
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
      UPDATE contact SET last_activity_at = v WHERE id = rid;
    WHEN 'deal'::regclass THEN
      PERFORM 1 FROM deal WHERE id = rid FOR UPDATE;
      v := last_activity_of_deal(rid);
      PERFORM set_config('margince.last_activity_move', 'on', true);
      UPDATE deal SET last_activity_at = v WHERE id = rid;
    WHEN 'company'::regclass THEN
      PERFORM 1 FROM company WHERE id = rid FOR UPDATE;
      v := last_activity_of_company(rid);
      PERFORM set_config('margince.last_activity_move', 'on', true);
      UPDATE company SET last_activity_at = v WHERE id = rid;
  END CASE;
  PERFORM set_config('margince.last_activity_move', 'off', true);
END;
$$;
