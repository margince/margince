-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- A contact serving notice still works for the company, so their mail sets its
-- clock. Both functions carry employment.IsCurrentSQL("r.ended_at") verbatim,
-- held there by backend/gates/accountreachcopies_test.go.
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
       AND (coalesce(r.employment_status, 'current') = 'current' AND (r.ended_at IS NULL OR CASE WHEN r.ended_precision = 'month' THEN (date_trunc('month', r.ended_at::date) + interval '1 month')::date ELSE r.ended_at END > current_date))
       AND r.archived_at IS NULL
       AND coalesce(r.started_at::timestamp AT TIME ZONE 'UTC', r.first_observed_at, a.occurred_at) <= a.occurred_at
  ) arms
$$;

CREATE OR REPLACE FUNCTION last_activity_reach(contact_ids uuid[], deal_ids uuid[], company_ids uuid[])
    RETURNS TABLE (kind text, id uuid)
    LANGUAGE sql STABLE
    AS $$
  SELECT 'contact', named FROM unnest(contact_ids) named WHERE named IS NOT NULL
  UNION SELECT 'deal', named FROM unnest(deal_ids) named WHERE named IS NOT NULL
  UNION SELECT 'company', named FROM unnest(company_ids) named WHERE named IS NOT NULL
  UNION SELECT 'company', d.company_id FROM deal d
         WHERE d.id = ANY (deal_ids) AND d.company_id IS NOT NULL
  UNION SELECT 'company', r.company_id FROM relationship r
         WHERE r.contact_id = ANY (contact_ids) AND r.kind = 'employment'
           AND (coalesce(r.employment_status, 'current') = 'current' AND (r.ended_at IS NULL OR CASE WHEN r.ended_precision = 'month' THEN (date_trunc('month', r.ended_at::date) + interval '1 month')::date ELSE r.ended_at END > current_date))
           AND r.archived_at IS NULL
$$;
