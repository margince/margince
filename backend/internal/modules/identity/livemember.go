// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import "github.com/margince/margince/backend/internal/shared/kernel/workforce"

// LiveMemberSQL is workforce.LiveMemberSQL under the name identity's callers
// use. TestEveryLiveMemberAliasIsALiteral holds that the alias is a literal.
func LiveMemberSQL(alias string) string {
	return workforce.LiveMemberSQL(alias)
}

// ActivatableMemberSQL is workforce.ActivatableMemberSQL, the set the
// set-password pair must reach and LiveMemberSQL excludes.
func ActivatableMemberSQL(alias string) string {
	return workforce.ActivatableMemberSQL(alias)
}

// DepartedMemberSQL is workforce.DepartedMemberSQL: left, not archived.
func DepartedMemberSQL(alias string) string {
	return workforce.DepartedMemberSQL(alias)
}
