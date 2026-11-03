-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- The validation of the guards the previous file added. A file of its own
-- because VALIDATE takes a lighter lock than the ADD, and only a separate
-- transaction lets writers through while it scans. None can fail: every row
-- written before the previous file holds NULL in both columns.
SET LOCAL lock_timeout = '3s';

ALTER TABLE deal_stage_history VALIDATE CONSTRAINT deal_stage_history_provenance_check;
ALTER TABLE deal_stage_history VALIDATE CONSTRAINT deal_stage_history_import_run_pairing;
