-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '5s';

-- The audit_log row written in the same transaction as this phase move. It
-- carries the actor type, the represented human, the passport and the client,
-- which changed_by (a flat principal id) cannot. No foreign key: audit_log is
-- append-only and owned by another module, and an erasure must not be blocked
-- by a history row. NULL marks a move whose audit row is not known; the data
-- task core/1792842001_phase_history_audit_links links the rows written before
-- this column existed.
ALTER TABLE project_phase_history
    ADD COLUMN audit_log_id uuid;
