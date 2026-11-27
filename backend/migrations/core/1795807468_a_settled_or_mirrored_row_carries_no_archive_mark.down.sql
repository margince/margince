-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '3s';

ALTER TABLE finance_payment ADD COLUMN archived_at timestamptz;
ALTER TABLE finance_invoice ADD COLUMN archived_at timestamptz;
ALTER TABLE finance_external_customer ADD COLUMN archived_at timestamptz;
ALTER INDEX conversation_claim_task_ix RENAME TO conversation_claim_task_next;
ALTER INDEX conversation_claim_contact_ix RENAME TO conversation_claim_contact_next;
ALTER INDEX conversation_claim_evidence_ux RENAME TO conversation_claim_evidence_next;
ALTER TABLE conversation_claim ADD COLUMN archived_at timestamptz;
