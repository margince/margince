-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '3s';

-- A permanent stop becomes the longest temporary one, so no reader's choice is lost.
UPDATE relationship_nudge_dismissal SET dismissed_until = set_at + interval '90 days'
 WHERE dismissed_until IS NULL;

ALTER TABLE relationship_nudge_dismissal ALTER COLUMN dismissed_until SET NOT NULL;

COMMENT ON TABLE relationship_nudge_dismissal IS
    'A relationship nudge one reader has dismissed until a moment. Per reader, never permanent.';
