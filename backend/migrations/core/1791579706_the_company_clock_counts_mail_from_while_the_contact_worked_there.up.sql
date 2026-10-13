-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- A company's last_activity_at counts a contact's mail only from the time the
-- contact worked there, as the account reach walk does. Without the bound, a
-- message from someone's previous job set the clock of the employer they have
-- now.
--
-- The added clause is employment.InPlaceAtSQL("r", "a.occurred_at") verbatim,
-- held there by backend/gates/accountreachcopies_test.go. An edge with neither
-- started_at nor first_observed_at stays unbounded.
--
-- No refold here: a stored clock moves when the company's activity, deals or
-- employment next change. trg_relationship_last_activity fires on every update
-- of an employment row, so a start date or a first observation written later
-- refolds the clock with it.
--
-- last_activity_reach keeps no bound. It names the records whose clock a write
-- may move, and must stay a superset of them: an activity whose date moves
-- across an employment's start changes that employer's clock.
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
