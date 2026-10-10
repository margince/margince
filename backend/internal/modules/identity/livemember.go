// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import "github.com/margince/margince/backend/internal/shared/kernel/workforce"

// LiveMemberSQL is what "someone who still works here" means, for a query that
// reads app_user. The definition is workforce.LiveMemberSQL, which a sibling
// module can import and this one's callers cannot all reach; this name is kept
// so every caller here and in compose reads one spelling.
//
// alias is formatted into the statement as an identifier, so it must be a
// compile-time literal. TestEveryLiveMemberAliasIsALiteral holds that.
func LiveMemberSQL(alias string) string {
	return workforce.LiveMemberSQL(alias)
}

// ActivatableMemberSQL is the set that may still BECOME active, which
// LiveMemberSQL excludes on purpose: the set-password pair must reach the
// invited member whose invitation redemption will accept. The definition is
// workforce.ActivatableMemberSQL.
func ActivatableMemberSQL(alias string) string {
	return workforce.ActivatableMemberSQL(alias)
}
