// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import "github.com/margince/margince/backend/internal/platform/database/storekit"

// LiveMemberSQL is what "someone who still works here" means, for a query that
// reads app_user.
//
// BOTH HALVES, and neither implies the other. Deactivating an account sets
// `status` and leaves `archived_at` NULL, so a predicate on `archived_at` alone
// goes on offering a departed colleague — as an intro route, an assignee, a name
// on a card. Archiving without deactivating is the mirror. A reader that tests
// one half has not asked the question.
//
// alias is the table's alias in the caller's query, or "" when it reads app_user
// unaliased. It is formatted into the statement as an IDENTIFIER, so it must be
// a compile-time literal and never anything off a request — an alias a caller
// chose is an injection with a placeholder's manners.
// TestEveryLiveMemberAliasIsALiteral holds that.
//
// WHO CAN CALL THIS: identity owns app_user, and a module never imports a
// sibling (ADR-0054 §3) — so `search`, `projects`, `dealrooms`, `people` and
// `activities` cannot, and spell the pair themselves. Those are ratified by name
// in TestOnlyOneSpellingOfALiveMember rather than left looking clean, which is
// the same shape the employment-currency census settled on. `compose` can import
// a module and does.
func LiveMemberSQL(alias string) string {
	prefix := ""
	if alias != "" {
		prefix = alias + "."
	}
	return storekit.SQLf("%sstatus = 'active' AND %sarchived_at IS NULL", prefix, prefix)
}

// ActivatableMemberSQL is the set that may still BECOME active: an invited seat
// that has never been entered, and an active one.
//
// It is NOT a second spelling of LiveMemberSQL and never answers "may this
// person act". Every read that gates access asks that question and takes
// LiveMemberSQL, which excludes an invited member on purpose: they have no
// password and no linked identity, so they sign in nowhere.
//
// It exists for the set-password pair, which must reach exactly the member
// LiveMemberSQL excludes. Redeeming an invitation IS the act of leaving
// 'invited', so redemption has to admit them; and issuance has to admit
// whomever redemption will, because a link minted for someone redemption
// refuses is dead on arrival, while a link refused to someone redemption would
// accept strands an expired invitation with no route back — that member has no
// password, so the forgot-password flow refuses them too.
//
// The archived half is carried for the same reason LiveMemberSQL carries it:
// archiving and deactivating are independent, so one half alone goes on
// offering a member the installation has already withdrawn.
func ActivatableMemberSQL(alias string) string {
	prefix := ""
	if alias != "" {
		prefix = alias + "."
	}
	return storekit.SQLf("%sstatus IN ('invited', 'active') AND %sarchived_at IS NULL", prefix, prefix)
}
