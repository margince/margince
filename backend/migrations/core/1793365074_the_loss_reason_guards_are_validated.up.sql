-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- The validation of the guards the previous file added. A file of its own
-- because VALIDATE takes a lighter lock than the ADD, and only a separate
-- transaction lets writers through while it scans. None can fail: the new
-- columns are empty, and the loss rule only got weaker.
SET LOCAL lock_timeout = '3s';

ALTER TABLE deal VALIDATE CONSTRAINT deal_lost_reason_code_fkey;
ALTER TABLE deal VALIDATE CONSTRAINT deal_lost_reason_code_only_when_lost;
ALTER TABLE deal VALIDATE CONSTRAINT deal_lost_reason_basis_check;
ALTER TABLE deal VALIDATE CONSTRAINT deal_lost_reason;
ALTER TABLE deal_stage_history VALIDATE CONSTRAINT deal_stage_history_lost_reason_code_fkey;
