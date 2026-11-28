-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '3s';

-- Nothing sets these marks: a mirror row is updated in place from its
-- accounting source.
ALTER TABLE finance_external_customer DROP COLUMN archived_at;
ALTER TABLE finance_invoice DROP COLUMN archived_at;
ALTER TABLE finance_payment DROP COLUMN archived_at;
