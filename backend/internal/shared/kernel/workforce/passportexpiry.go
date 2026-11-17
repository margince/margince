// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package workforce

// PassportUnexpiredSQL is whether passport alias has not run out at the SQL
// expression at. A null expiry is a passport minted to never expire. Every SQL
// reader of the expiry asks this rule; identity's PassportRow.live is its Go twin.
func PassportUnexpiredSQL(alias, at string) string {
	return `(` + alias + `.expires_at IS NULL OR ` + at + ` < ` + alias + `.expires_at)`
}
