-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- NULL means no passport stands behind the rule. No older rule was an agent's, because
-- the route took only a signed-in user's session until this column shipped.
SET LOCAL lock_timeout = '3s';

ALTER TABLE automation
    ADD COLUMN setter_passport_id uuid,
    ADD CONSTRAINT automation_setter_passport_id_fkey
        FOREIGN KEY (setter_passport_id) REFERENCES passport(id) ON DELETE SET NULL NOT VALID,
    DROP CONSTRAINT automation_paused_reason_check,
    ADD CONSTRAINT automation_paused_reason_check
        CHECK (paused_reason IN ('list_archived', 'list_invalid', 'list_unavailable', 'burst', 'passport_revoked')) NOT VALID;
