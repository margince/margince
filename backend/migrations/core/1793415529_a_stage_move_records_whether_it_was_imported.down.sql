-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '3s';

ALTER TABLE deal_stage_history
    DROP CONSTRAINT deal_stage_history_import_run_pairing,
    DROP CONSTRAINT deal_stage_history_provenance_check,
    DROP COLUMN import_run_id,
    DROP COLUMN provenance;
