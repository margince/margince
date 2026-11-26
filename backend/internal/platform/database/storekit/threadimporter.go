// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

// ThreadImportedByClause is the test for "this seat's own message of a thread".
// The row carries a thread key and is live and unrestricted, and the seat holds
// an import row for it. The thread audience write acts on these rows. Every
// control that offers that write renders this clause, so the offer cannot
// differ from what the write finds. userRef is a bound placeholder such as "$2".
func ThreadImportedByClause(alias, userRef string) string {
	return "(" + alias + ".thread_key IS NOT NULL AND " + alias + ".archived_at IS NULL AND " +
		alias + ".restricted_at IS NULL AND EXISTS (SELECT 1 FROM capture_import i WHERE i.activity_id = " +
		alias + ".id AND i.user_id = " + userRef + "))"
}
