-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '3s';

-- Before the clearing update, so no pause can add the reason behind it.
LOCK TABLE automation IN ACCESS EXCLUSIVE MODE;

UPDATE automation SET paused_reason = NULL WHERE paused_reason = 'passport_revoked';

ALTER TABLE automation
    DROP COLUMN setter_passport_id,
    DROP CONSTRAINT automation_paused_reason_check,
    ADD CONSTRAINT automation_paused_reason_check
        CHECK (paused_reason IN ('list_archived', 'list_invalid', 'list_unavailable', 'burst')) NOT VALID;
