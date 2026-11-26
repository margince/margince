-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '3s';

ALTER TABLE consent_qualifying_event
    DROP CONSTRAINT consent_qualifying_event_one_subject,
    ADD CONSTRAINT consent_qualifying_event_one_subject
        CHECK (num_nonnulls(contact_id, lead_id) = 1) NOT VALID;
