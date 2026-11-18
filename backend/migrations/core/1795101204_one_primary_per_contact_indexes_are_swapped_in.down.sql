-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '3s';

DROP INDEX IF EXISTS uq_contact_email_primary;
CREATE UNIQUE INDEX IF NOT EXISTS uq_contact_email_primary ON contact_email (contact_id, email_type) WHERE is_primary AND archived_at IS NULL;
DROP INDEX IF EXISTS uq_contact_phone_primary;
CREATE UNIQUE INDEX IF NOT EXISTS uq_contact_phone_primary ON contact_phone (contact_id, phone_type) WHERE is_primary AND archived_at IS NULL;
