// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

// The generation binds an opaque provider id to the account that supplied it.
// Every argument is an internally derived SQL placeholder.
func ProviderMessageKeysSQL(seat, provider, message string) string {
	return ConnectionMessageKeysSQL("c.user_id = "+seat+" AND c.provider = "+provider, message)
}

// ConnectionMessageKeysSQL is ProviderMessageKeysSQL for the live connection
// match names over alias c, such as a shared mailbox by its id.
func ConnectionMessageKeysSQL(match, message string) string {
	return "ARRAY(SELECT c.provider || ':' || c.generation || ':' || " + message +
		" FROM capture_connection c WHERE " + match +
		" AND c.archived_at IS NULL AND c.status IN ('connected','error') AND " + message + " <> '')"
}
