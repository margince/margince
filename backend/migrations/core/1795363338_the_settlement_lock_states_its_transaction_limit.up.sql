-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '2s';

-- Every row written to activity, activity_participant, activity_link or
-- contact_email takes one shared advisory lock per conversation key through
-- this function, held until commit. Each lock uses a slot in the shared lock
-- table, which holds max_locks_per_transaction * (max_connections +
-- max_prepared_transactions) locks for the whole server. A transaction past
-- that fails with out of shared memory (SQLSTATE 53200). So a bulk writer
-- commits in batches of a few thousand rows at most.
COMMENT ON FUNCTION settlement_evidence_share(bigint[]) IS
    'Takes one shared advisory lock per distinct conversation key, held until commit. A transaction writing more rows to activity, activity_participant, activity_link or contact_email than the shared lock table holds fails with out of shared memory, so bulk writers commit in batches of a few thousand rows.';
