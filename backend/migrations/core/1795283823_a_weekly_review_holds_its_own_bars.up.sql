-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- The bars of a week's bridge were rows of a child table that nothing read
-- apart from its review. They become one array on the review, each element
-- naming its horizon, its bar and the signed amount. NULL means no bar was
-- frozen. The writer in compose/weekly holds the two vocabularies.
SET LOCAL lock_timeout = '3s';

ALTER TABLE weekly_review ADD COLUMN outlook_movement jsonb;

UPDATE weekly_review r
   SET outlook_movement = m.bars
  FROM (SELECT weekly_review_id,
               jsonb_agg(jsonb_build_object('period_kind', period_kind, 'bar', bar, 'delta_minor', delta_minor)
                         ORDER BY period_kind, bar) AS bars
          FROM weekly_review_movement
         GROUP BY weekly_review_id) m
 WHERE r.id = m.weekly_review_id;

DROP TABLE weekly_review_movement;
