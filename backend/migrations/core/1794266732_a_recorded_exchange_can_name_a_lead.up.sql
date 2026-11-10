-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '3s';

ALTER TABLE consent_qualifying_event
    ADD COLUMN lead_id uuid REFERENCES lead(id) ON DELETE CASCADE,
    ALTER COLUMN contact_id DROP NOT NULL,
    ADD CONSTRAINT consent_qualifying_event_one_subject
        CHECK (num_nonnulls(contact_id, lead_id) = 1) NOT VALID;

CREATE INDEX consent_qualifying_event_lead_idx
    ON consent_qualifying_event (lead_id, occurred_at DESC)
    WHERE lead_id IS NOT NULL;
