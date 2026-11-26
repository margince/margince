-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- The open question tasks of a live colleague with a full seat move into the
-- queue: one pending row per deal the task offered, for the assignee, and the
-- task is done. A task of anyone else stays as it was. There is at most one
-- open question per conversation and colleague, so the set is small.
SET LOCAL lock_timeout = '5s';

-- A task written before tasks recorded their deals offers the open deals of
-- its project, else of its company, as the decline read them.
WITH question AS (
    SELECT q.id, q.source_activity_id AS mail, q.assignee_id, q.subject, q.captured_by, q.raw->'offered_deal_ids' AS offered
      FROM activity q JOIN app_user u ON u.id = q.assignee_id
     WHERE q.kind = 'task' AND q.source_system = 'which_deal' AND NOT q.is_done
       AND q.archived_at IS NULL AND q.restricted_at IS NULL AND q.source_activity_id IS NOT NULL
       AND u.is_agent = false AND u.status = 'active' AND u.archived_at IS NULL AND u.seat_type = 'full'
), offer AS (
    SELECT q.id AS question, q.mail, q.assignee_id, q.subject, q.captured_by, d.id AS deal
      FROM question q JOIN deal d ON d.status = 'open' AND d.archived_at IS NULL AND CASE
        WHEN jsonb_typeof(q.offered) = 'array'
          THEN d.id::text IN (SELECT jsonb_array_elements_text(q.offered))
        ELSE EXISTS (SELECT 1 FROM activity_link ql WHERE ql.activity_id = q.id
               AND ((ql.entity_type = 'project' AND d.project_id = ql.project_id)
                 OR (ql.entity_type = 'company' AND d.company_id = ql.company_id
                     AND NOT EXISTS (SELECT 1 FROM activity_link qp
                                      WHERE qp.activity_id = q.id AND qp.entity_type = 'project'))))
        END
)
INSERT INTO activity_link_suggestion (activity_id, entity_type, entity_id, unit, for_user, reason, score, captured_by)
SELECT o.mail, 'deal', o.deal, NULL, o.assignee_id,
       left(coalesce(nullif(btrim(o.subject), ''), 'Which deal is this about?'), 200),
       round(1.0 / count(*) OVER (PARTITION BY o.question), 4), o.captured_by
  FROM offer o
ON CONFLICT (activity_id, entity_type, entity_id, for_user) WHERE for_user IS NOT NULL DO NOTHING;

UPDATE activity q SET is_done = true, done_at = now()
  FROM app_user u
 WHERE u.id = q.assignee_id AND q.kind = 'task' AND q.source_system = 'which_deal' AND NOT q.is_done
   AND q.archived_at IS NULL AND q.restricted_at IS NULL AND q.source_activity_id IS NOT NULL
   AND u.is_agent = false AND u.status = 'active' AND u.archived_at IS NULL AND u.seat_type = 'full';
