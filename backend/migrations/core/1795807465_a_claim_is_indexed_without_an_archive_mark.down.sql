-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- pgmigrate:no-transaction
SET lock_timeout = '3s';
DROP INDEX CONCURRENTLY IF EXISTS idx_conversation_claim_contact;
CREATE INDEX CONCURRENTLY idx_conversation_claim_contact ON conversation_claim (contact_id);
DROP INDEX CONCURRENTLY IF EXISTS conversation_claim_evidence_ux;
CREATE UNIQUE INDEX CONCURRENTLY conversation_claim_evidence_ux ON conversation_claim (contact_id, source_activity_id, evidence_fingerprint) WHERE archived_at IS NULL;
DROP INDEX CONCURRENTLY IF EXISTS conversation_claim_evidence_next;
DROP INDEX CONCURRENTLY IF EXISTS conversation_claim_contact_ix;
CREATE INDEX CONCURRENTLY conversation_claim_contact_ix ON conversation_claim (contact_id, kind) WHERE archived_at IS NULL;
DROP INDEX CONCURRENTLY IF EXISTS conversation_claim_contact_next;
DROP INDEX CONCURRENTLY IF EXISTS conversation_claim_task_ix;
CREATE INDEX CONCURRENTLY conversation_claim_task_ix ON conversation_claim (task_activity_id) WHERE task_activity_id IS NOT NULL AND archived_at IS NULL;
DROP INDEX CONCURRENTLY IF EXISTS conversation_claim_task_next;
RESET lock_timeout;
