-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- The team's frozen coverage records whether the standing call it reads had an
-- open input-check finding when the week closed.
--
-- Additive. NULL means the call was not checked: a coverage frozen before this
-- column, or one that reads no manager's call. It is never read as "current".
SET LOCAL lock_timeout = '3s';

ALTER TABLE team_weekly_review_coverage ADD COLUMN call_needs_review boolean;
