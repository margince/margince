-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- A link suggestion may come from Margince itself: the "Which deal is this
-- about?" question capture asks when several open deals fit a mail. That
-- question is for the one colleague the mailbox confirms is responsible, so
-- the row names its reader instead of a unit. A unit's row names no reader.
-- Both constraints are added NOT VALID; a later migration validates them.
SET LOCAL lock_timeout = '5s';

ALTER TABLE activity_link_suggestion
    ALTER COLUMN unit DROP NOT NULL,
    ADD COLUMN IF NOT EXISTS for_user uuid,
    ADD CONSTRAINT activity_link_suggestion_for_user_fkey FOREIGN KEY (for_user)
        REFERENCES app_user(id) ON DELETE CASCADE NOT VALID,
    ADD CONSTRAINT activity_link_suggestion_producer CHECK ((unit IS NULL) = (for_user IS NOT NULL)) NOT VALID;
