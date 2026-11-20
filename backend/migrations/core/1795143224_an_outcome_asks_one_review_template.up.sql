-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '3s';

-- A closing asks one review, so an outcome has at most one active template.
-- Adding a template or turning a retired one back on retires the one asked
-- before. An installation that already holds two keeps the oldest, the one the
-- review writer picked until now.
UPDATE activity_review_template t
   SET active = false
 WHERE t.active AND t.archived_at IS NULL
   AND EXISTS (SELECT 1 FROM activity_review_template older
                WHERE older.outcome = t.outcome AND older.active AND older.archived_at IS NULL
                  AND (older.created_at, older.id) < (t.created_at, t.id));

CREATE UNIQUE INDEX IF NOT EXISTS activity_review_template_one_active_per_outcome
    ON activity_review_template (outcome) WHERE active AND archived_at IS NULL;
