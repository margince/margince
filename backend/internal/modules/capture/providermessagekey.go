// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// The generation binds an opaque provider id to the account that supplied it.
// Every argument is an internally derived SQL placeholder.
func providerMessageKeysSQL(seat, provider, message string) string {
	return "ARRAY(SELECT c.provider || ':' || c.generation || ':' || " + message +
		" FROM capture_connection c WHERE c.user_id = " + seat + " AND c.provider = " + provider +
		" AND c.archived_at IS NULL AND c.status IN ('connected','error') AND " + message + " <> '')"
}
