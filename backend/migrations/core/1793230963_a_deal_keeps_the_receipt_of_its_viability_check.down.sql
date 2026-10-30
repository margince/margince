-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '3s';

ALTER TABLE assurance_exception
    DROP COLUMN task_activity_id,
    DROP COLUMN episode;

DROP TABLE assurance_deal_receipt;
