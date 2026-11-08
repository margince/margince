-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Validate the check the previous file added NOT VALID. Its own file, so the
-- scan runs under SHARE UPDATE EXCLUSIVE. It cannot fail: the rule only got
-- looser.
SET LOCAL lock_timeout = '3s';

ALTER TABLE deal_stage_history VALIDATE CONSTRAINT deal_stage_history_provenance_check;
