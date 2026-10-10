// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package workforce is what "someone who still works here" means, in one place
// every module can reach.
//
// identity owns app_user and a module never imports a sibling, so every module
// that joins app_user either spelled the pair out itself or sat in a ratified
// list. The definition is a domain rule with no state, which is what the shared
// kernel holds; identity.LiveMemberSQL delegates here so its callers keep one
// name. stdlib only, as the tier requires.
package workforce

import "fmt"

// LiveMemberSQL is the predicate on app_user for a seat that has not left.
//
// BOTH HALVES, and neither implies the other. Deactivating an account sets
// `status` and leaves `archived_at` NULL, so a predicate on `archived_at` alone
// goes on offering a departed colleague as an intro route, an assignee, a name
// on a card. Archiving without deactivating is the mirror.
//
// alias is the table's alias in the caller's query, or "" when it reads app_user
// unaliased. It is formatted into the statement as an identifier, so it must be
// a compile-time literal and never anything off a request.
func LiveMemberSQL(alias string) string {
	prefix := ""
	if alias != "" {
		prefix = alias + "."
	}
	return fmt.Sprintf("%sstatus = 'active' AND %sarchived_at IS NULL", prefix, prefix)
}

// ActivatableMemberSQL is the set that may still BECOME active: an invited seat
// that has never been entered, and an active one.
//
// It is NOT a second spelling of LiveMemberSQL and never answers "may this
// contact act". Every read that gates access takes LiveMemberSQL, which
// excludes an invited member on purpose: they have no password and no linked
// identity, so they sign in nowhere. It exists for the set-password pair, and
// for a task created on an invited colleague's behalf, which waits in their
// Worklist until they sign in.
//
// The archived half is carried for the same reason LiveMemberSQL carries it:
// archiving and deactivating are independent, so one half alone goes on
// offering a member the installation has already withdrawn.
func ActivatableMemberSQL(alias string) string {
	prefix := ""
	if alias != "" {
		prefix = alias + "."
	}
	return fmt.Sprintf("%sstatus IN ('invited', 'active') AND %sarchived_at IS NULL", prefix, prefix)
}
