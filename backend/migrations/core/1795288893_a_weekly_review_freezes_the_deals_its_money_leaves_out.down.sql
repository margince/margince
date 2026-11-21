-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '3s';

ALTER TABLE team_weekly_review DROP COLUMN IF EXISTS pipeline_gaps;
ALTER TABLE weekly_review DROP COLUMN IF EXISTS pipeline_gaps;
