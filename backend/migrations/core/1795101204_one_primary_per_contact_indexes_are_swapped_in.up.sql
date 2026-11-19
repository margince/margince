-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- The swap holds each table's lock for a drop and a rename, both instant.
SET LOCAL lock_timeout = '3s';

DROP INDEX IF EXISTS uq_contact_email_primary;
ALTER INDEX uq_contact_email_primary_next RENAME TO uq_contact_email_primary;
DROP INDEX IF EXISTS uq_contact_phone_primary;
ALTER INDEX uq_contact_phone_primary_next RENAME TO uq_contact_phone_primary;
