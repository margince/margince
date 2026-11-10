-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '3s';

ALTER TABLE mail_draft
    DROP CONSTRAINT IF EXISTS mail_draft_communication_context,
    DROP CONSTRAINT IF EXISTS mail_draft_schedule_pair,
    DROP COLUMN IF EXISTS scheduled_tz,
    DROP COLUMN IF EXISTS scheduled_at,
    DROP COLUMN IF EXISTS communication_context,
    DROP COLUMN IF EXISTS attachment_ids;
