// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

import "fmt"

// A merged account remains an immutable author. Its surviving member inherits
// that identity's correspondence access, while grants still use the live seat.
//
// The family is one SQL function: a recursive CTE spelled into each clause was
// planned once per copy, and a connection keeps the planner's memory.
func memberIdentitySQL(position int) string {
	return fmt.Sprintf("SELECT unnest(member_identity_family($%d::uuid)) AS id", position)
}

func memberIdentitySetSQL(position int) string {
	return fmt.Sprintf("SELECT unnest(member_identity_family(root)) AS id FROM unnest($%d::uuid[]) root", position)
}

func memberAuthorshipSQL(alias string, memberPosition int, arg func(any) int) string {
	return fmt.Sprintf(`EXISTS (SELECT 1 FROM (%s) author_identity WHERE %s.captured_by LIKE $%d || author_identity.id::text)`, memberIdentitySQL(memberPosition), alias, arg("%:"))
}
