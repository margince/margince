-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '5s';

-- The narrowed state CHECK was added NOT VALID one migration earlier. That
-- migration moved every 'awaiting_decision' row, so this succeeds.
ALTER TABLE communication_review VALIDATE CONSTRAINT communication_review_state;
