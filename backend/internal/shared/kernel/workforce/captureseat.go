// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package workforce

// CaptureSeatSQL retains departed colleagues' correspondence but excludes retired identities.
// A revoked invitation remains indistinguishable from an imported inactive colleague.
func CaptureSeatSQL(alias string) string {
	return alias + ".archived_at IS NULL AND " + alias + ".removed_at IS NULL"
}
