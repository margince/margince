-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '3s';

-- Nothing sets these marks. A claim settles through its status, and a mirror
-- row is updated in place from its accounting source.
ALTER TABLE conversation_claim DROP COLUMN archived_at;
ALTER INDEX conversation_claim_evidence_next RENAME TO conversation_claim_evidence_ux;
ALTER INDEX conversation_claim_contact_next RENAME TO conversation_claim_contact_ix;
ALTER INDEX conversation_claim_task_next RENAME TO conversation_claim_task_ix;
ALTER TABLE finance_external_customer DROP COLUMN archived_at;
ALTER TABLE finance_invoice DROP COLUMN archived_at;
ALTER TABLE finance_payment DROP COLUMN archived_at;
