-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Rows a sync wrote stay; the narrower check holds new rows only.
SET LOCAL lock_timeout = '3s';

ALTER TABLE deal_stage_history
    DROP CONSTRAINT deal_stage_history_provenance_check,
    ADD CONSTRAINT deal_stage_history_provenance_check
        CHECK (provenance IS NULL OR provenance IN ('recorded', 'imported')) NOT VALID;
