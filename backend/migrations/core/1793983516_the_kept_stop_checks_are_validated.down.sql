-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Put both checks back present and unvalidated, as this migration found them.
-- Postgres has no ALTER that un-validates, so each is dropped and re-added.
SET LOCAL lock_timeout = '3s';

ALTER TABLE communication_suppression
    DROP CONSTRAINT communication_suppression_recorded_through,
    ADD CONSTRAINT communication_suppression_recorded_through
        CHECK (recorded_through IN ('colleague', 'agent', 'subject', 'machine', 'unknown')) NOT VALID,
    DROP CONSTRAINT communication_suppression_names_a_target,
    ADD CONSTRAINT communication_suppression_names_a_target
        CHECK (contact_id IS NOT NULL OR lead_id IS NOT NULL OR address IS NOT NULL
               OR address_hash IS NOT NULL) NOT VALID;
