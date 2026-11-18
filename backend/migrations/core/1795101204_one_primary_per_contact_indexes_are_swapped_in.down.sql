-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Back to the state before the swap: the per-contact indexes wait under their
-- temporary names and the per-type ones hold the slot.
SET LOCAL lock_timeout = '3s';

ALTER INDEX IF EXISTS uq_contact_email_primary RENAME TO uq_contact_email_primary_next;
CREATE UNIQUE INDEX IF NOT EXISTS uq_contact_email_primary ON contact_email (contact_id, email_type) WHERE is_primary AND archived_at IS NULL;
ALTER INDEX IF EXISTS uq_contact_phone_primary RENAME TO uq_contact_phone_primary_next;
CREATE UNIQUE INDEX IF NOT EXISTS uq_contact_phone_primary ON contact_phone (contact_id, phone_type) WHERE is_primary AND archived_at IS NULL;
