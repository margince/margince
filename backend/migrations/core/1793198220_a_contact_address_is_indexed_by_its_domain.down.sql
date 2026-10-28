-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion
-- pgmigrate:no-transaction
DROP INDEX CONCURRENTLY IF EXISTS idx_contact_email_domain_reversed;
