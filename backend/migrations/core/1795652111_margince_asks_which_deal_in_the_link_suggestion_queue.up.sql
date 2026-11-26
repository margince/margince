-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- A link suggestion may come from Margince itself: the "Which deal is this
-- about?" question capture asks when several open deals fit a mail. That
-- question is for the one colleague the mailbox confirms is responsible, so
-- the row names its reader instead of a unit. A unit's row names no reader.
--
-- The open question tasks move into the queue: one pending row per deal the
-- task offered, for the task's assignee, and the task is done. There is at
-- most one open question per conversation and colleague, so the set is small.
SET LOCAL lock_timeout = '5s';

ALTER TABLE activity_link_suggestion
    ALTER COLUMN unit DROP NOT NULL,
    ADD COLUMN IF NOT EXISTS for_user uuid,
    ADD CONSTRAINT activity_link_suggestion_for_user_fkey FOREIGN KEY (for_user)
        REFERENCES app_user(id) ON DELETE CASCADE,
    ADD CONSTRAINT activity_link_suggestion_producer CHECK ((unit IS NULL) = (for_user IS NOT NULL));

CREATE INDEX IF NOT EXISTS idx_activity_link_suggestion_for_user
    ON activity_link_suggestion (for_user) WHERE for_user IS NOT NULL;

-- A task written before tasks recorded their deals offers the open deals of
-- its project, else of its company, as the decline read them.
WITH question AS (
    SELECT q.id, q.source_activity_id AS mail, q.assignee_id, q.subject, q.captured_by, q.raw->'offered_deal_ids' AS offered
      FROM activity q
     WHERE q.kind = 'task' AND q.source_system = 'which_deal' AND NOT q.is_done
       AND q.archived_at IS NULL AND q.restricted_at IS NULL
       AND q.source_activity_id IS NOT NULL AND q.assignee_id IS NOT NULL
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
ON CONFLICT (activity_id, entity_type, entity_id) DO NOTHING;

UPDATE activity SET is_done = true, done_at = now()
 WHERE kind = 'task' AND source_system = 'which_deal' AND NOT is_done
   AND archived_at IS NULL AND restricted_at IS NULL;
