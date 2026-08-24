// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import "github.com/gradionhq/margince/backend/internal/platform/database/storekit"

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
