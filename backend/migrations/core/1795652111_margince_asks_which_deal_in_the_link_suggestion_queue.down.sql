-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- The queue goes back to holding only what units proposed. Margince's own deal
-- questions are deleted; the tasks the up closed stay done.
SET LOCAL lock_timeout = '5s';

DELETE FROM activity_link_suggestion WHERE unit IS NULL;

DROP INDEX IF EXISTS idx_activity_link_suggestion_for_user;
DROP INDEX IF EXISTS uq_activity_link_suggestion_question;
DROP INDEX IF EXISTS uq_activity_link_suggestion_unit_target;
CREATE UNIQUE INDEX IF NOT EXISTS uq_activity_link_suggestion_target
    ON activity_link_suggestion (activity_id, entity_type, entity_id);

ALTER TABLE activity_link_suggestion
    DROP CONSTRAINT IF EXISTS activity_link_suggestion_producer,
    DROP CONSTRAINT IF EXISTS activity_link_suggestion_for_user_fkey,
    DROP COLUMN IF EXISTS for_user,
    ALTER COLUMN unit SET NOT NULL;
