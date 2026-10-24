-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- A reader may stop following a quiet contact with no end date. NULL in
-- dismissed_until is that permanent stop; the forward check still holds every
-- dated row, because a CHECK over NULL passes.
SET LOCAL lock_timeout = '3s';

ALTER TABLE relationship_nudge_dismissal ALTER COLUMN dismissed_until DROP NOT NULL;

COMMENT ON TABLE relationship_nudge_dismissal IS
    'A relationship nudge one reader has dismissed until a moment, or for good (NULL). Per reader, reversible.';
