-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- The two basis checks added NOT VALID by the file before this one are validated
-- here, in their own transaction, so the scan runs under SHARE UPDATE EXCLUSIVE.
-- The file before mapped every row holding the retired value, and nothing writes it.
SET LOCAL lock_timeout = '5s';

ALTER TABLE communication_decision VALIDATE CONSTRAINT communication_decision_basis;
ALTER TABLE communication_basis VALIDATE CONSTRAINT communication_basis_kind;
