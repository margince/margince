-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- The team's frozen coverage records whether the standing call it reads had an
-- open input-check finding when the week closed.
--
-- Additive. A coverage frozen before this column reads false, as it always did.
SET LOCAL lock_timeout = '3s';

ALTER TABLE team_weekly_review_coverage ADD COLUMN call_needs_review boolean NOT NULL DEFAULT false;
