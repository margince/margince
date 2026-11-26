-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Validate the dunning level range the migration before this one added NOT
-- VALID. Its own file, so the scan runs under SHARE UPDATE EXCLUSIVE.
SET LOCAL lock_timeout = '3s';

ALTER TABLE finance_invoice VALIDATE CONSTRAINT finance_invoice_dunning_level_range;
