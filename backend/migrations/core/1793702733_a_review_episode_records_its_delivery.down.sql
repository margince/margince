-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '3s';

ALTER TABLE assurance_exception
    DROP COLUMN delivered_assignee_id,
    DROP COLUMN delivery_attempts,
    DROP COLUMN delivery_state;
