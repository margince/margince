-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '3s';

ALTER TABLE communication_decision
  DROP CONSTRAINT IF EXISTS communication_decision_ruleset_digest_shape;

ALTER TABLE communication_decision DROP COLUMN IF EXISTS ruleset_digest;
