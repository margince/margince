-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion
-- pgmigrate:no-transaction
-- The index a company's link candidates are found by: live addresses on the
-- company's folded domains. Built concurrently, so writes are never blocked, and
-- the first drop clears an invalid index a failed build left behind.
DROP INDEX CONCURRENTLY IF EXISTS idx_contact_email_domain_folded;
CREATE INDEX CONCURRENTLY idx_contact_email_domain_folded ON contact_email (email_domain_folded)
    WHERE archived_at IS NULL;
