-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- The queue goes back to holding only what units proposed. The down of the
-- task move deleted Margince's own rows first.
SET LOCAL lock_timeout = '5s';

DELETE FROM activity_link_suggestion WHERE unit IS NULL;

ALTER TABLE activity_link_suggestion
    DROP CONSTRAINT IF EXISTS activity_link_suggestion_producer,
    DROP CONSTRAINT IF EXISTS activity_link_suggestion_for_user_fkey,
    DROP COLUMN IF EXISTS for_user,
    ALTER COLUMN unit SET NOT NULL;
