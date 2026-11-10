-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '3s';

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM consent_qualifying_event WHERE lead_id IS NOT NULL) THEN
        RAISE EXCEPTION 'Cannot roll back while recorded lead exchanges exist'
            USING ERRCODE = 'check_violation';
    END IF;
END $$;

DROP INDEX consent_qualifying_event_lead_idx;
ALTER TABLE consent_qualifying_event
    DROP CONSTRAINT consent_qualifying_event_one_subject,
    DROP COLUMN lead_id,
    ALTER COLUMN contact_id SET NOT NULL;
