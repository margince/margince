-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- pgmigrate:no-transaction
-- The claim indexes lose their archive predicate before the column goes, so the
-- evidence upsert always has a unique index to infer. Without its predicate the
-- (contact_id, kind) index covers every claim, so the contact_id index goes.
SET lock_timeout = '3s';
DROP INDEX CONCURRENTLY IF EXISTS conversation_claim_evidence_next;
CREATE UNIQUE INDEX CONCURRENTLY conversation_claim_evidence_next ON conversation_claim (contact_id, source_activity_id, evidence_fingerprint);
DROP INDEX CONCURRENTLY IF EXISTS conversation_claim_contact_next;
CREATE INDEX CONCURRENTLY conversation_claim_contact_next ON conversation_claim (contact_id, kind);
DROP INDEX CONCURRENTLY IF EXISTS conversation_claim_task_next;
CREATE INDEX CONCURRENTLY conversation_claim_task_next ON conversation_claim (task_activity_id) WHERE task_activity_id IS NOT NULL;
DROP INDEX CONCURRENTLY IF EXISTS idx_conversation_claim_contact;
RESET lock_timeout;
