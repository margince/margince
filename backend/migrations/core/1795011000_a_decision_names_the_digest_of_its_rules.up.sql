-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- A decision names the digest of the source its pack was compiled from, beside
-- the pack's version. NULL for a pack written by hand, for a fold of two packs,
-- and on every row written before this column.
--
-- The digest travels only with a single jurisdiction and its version. The
-- CHECK is added NOT VALID so the add takes no scan under its lock; the next
-- migration validates it.
SET LOCAL lock_timeout = '3s';

ALTER TABLE communication_decision ADD COLUMN IF NOT EXISTS ruleset_digest text;

ALTER TABLE communication_decision
  ADD CONSTRAINT communication_decision_ruleset_digest_shape CHECK (
    ruleset_digest IS NULL
    OR (ruleset_digest ~ '^[0-9a-f]{64}$' AND ruleset_version IS NOT NULL
        AND coalesce(array_length(ruleset_codes, 1), 0) = 1)
  ) NOT VALID;
