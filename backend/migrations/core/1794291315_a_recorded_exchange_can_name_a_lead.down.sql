-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '3s';

ALTER TABLE consent_qualifying_event
    ALTER COLUMN contact_id SET NOT NULL;

DROP INDEX consent_qualifying_event_lead_idx;
ALTER TABLE consent_qualifying_event
    DROP CONSTRAINT consent_qualifying_event_one_subject,
    DROP COLUMN lead_id;
