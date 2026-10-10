// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package workforce is what "someone who still works here" means, in one place
// every module can reach.
//
// identity owns app_user and a module never imports a sibling. So each module
// that joined app_user spelled the pair out itself. The definition is a domain
// rule with no state, which is what the shared kernel holds. identity delegates
// here so its callers keep one name. stdlib only, as the tier requires.
package workforce

import "fmt"

// LiveMemberSQL is the predicate on app_user for a seat that has not left.
//
// It names both halves, and neither implies the other. Deactivating an account
// sets `status` and leaves `archived_at` NULL. A test on `archived_at` alone
// offers a departed colleague as an intro route, an assignee or a name on a card.
//
// alias is the table's alias in the caller's query, or "" for an unaliased read.
// It is formatted in as an identifier, so it must be a compile-time literal.
func LiveMemberSQL(alias string) string {
	prefix := ""
	if alias != "" {
		prefix = alias + "."
	}
	return fmt.Sprintf("%sstatus = 'active' AND %sarchived_at IS NULL", prefix, prefix)
}

// ActivatableMemberSQL is the set that may still become active: an invited seat
// that has never been entered, and an active one.
//
// It is not a second spelling of LiveMemberSQL. Every read that gates access
// takes LiveMemberSQL, which excludes an invited member because they have no
// password and no linked identity. This set serves the set-password pair, and a
// task created for an invited colleague, which waits until they sign in.
//
// It keeps the archived half for the reason LiveMemberSQL does: archiving and
// deactivating are independent.
func ActivatableMemberSQL(alias string) string {
	prefix := ""
	if alias != "" {
		prefix = alias + "."
	}
	return fmt.Sprintf("%sstatus IN ('invited', 'active') AND %sarchived_at IS NULL", prefix, prefix)
}
