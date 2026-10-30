-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- The validation of the widened lifecycle check. A file of its own because
-- VALIDATE takes a lighter lock than the ADD, and only a separate transaction
-- lets writers through while it scans. No row can fail: the list only grew.
SET LOCAL lock_timeout = '3s';

ALTER TABLE company VALIDATE CONSTRAINT company_lifecycle_check;
