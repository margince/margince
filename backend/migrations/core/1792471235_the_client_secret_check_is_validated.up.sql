-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Validates the CHECK 1792471234 added, in a transaction of its own so the
-- scan runs under SHARE UPDATE EXCLUSIVE. Every stored row passes: no row held
-- a secret before that migration.
SET LOCAL lock_timeout = '3s';

ALTER TABLE oauth_client VALIDATE CONSTRAINT oauth_client_secret_is_admin_only;
