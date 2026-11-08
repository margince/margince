-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- A stage move another CRM made, which an extension mirrored in, says so. It
-- names no import run, so the run pairing already holds it. Every existing row
-- holds the wider check, so it is added without a scan; the next file
-- validates it.
SET LOCAL lock_timeout = '3s';

ALTER TABLE deal_stage_history
    DROP CONSTRAINT deal_stage_history_provenance_check,
    ADD CONSTRAINT deal_stage_history_provenance_check
        CHECK (provenance IS NULL OR provenance IN ('recorded', 'imported', 'synced')) NOT VALID;
