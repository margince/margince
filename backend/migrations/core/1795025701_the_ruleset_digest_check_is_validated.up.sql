-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Validates the digest check the previous migration added NOT VALID. A file of
-- its own, so the scan takes only SHARE UPDATE EXCLUSIVE and writers go through.
SET LOCAL lock_timeout = '3s';

ALTER TABLE communication_decision VALIDATE CONSTRAINT communication_decision_ruleset_digest_shape;
