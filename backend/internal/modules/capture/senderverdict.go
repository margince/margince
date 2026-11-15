// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// A late answer to an older question cannot replace a newer resolved one.
// alias is a compile-time literal naming a ledger row.
func latestResolvedSenderVerdict(alias string) string {
	return alias + `.resolved_at IS NOT NULL AND NOT EXISTS (
		SELECT 1 FROM capture_pending_counterparty newer
		 WHERE newer.owner_id = ` + alias + `.owner_id AND newer.email = ` + alias + `.email
		   AND newer.resolved_at IS NOT NULL
		   AND (newer.created_at, newer.id) > (` + alias + `.created_at, ` + alias + `.id))`
}
