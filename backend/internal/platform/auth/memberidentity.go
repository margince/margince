// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

import "fmt"

// A merged account remains an immutable author. Its surviving member inherits
// that identity's correspondence access, while grants still use the live seat.
func memberIdentitySQL(position int) string {
	return memberIdentityFamilySQL(fmt.Sprintf("SELECT $%d::uuid AS id", position))
}

func memberIdentitySetSQL(position int) string {
	return memberIdentityFamilySQL(fmt.Sprintf("SELECT unnest($%d::uuid[]) AS id", position))
}

func memberIdentityFamilySQL(roots string) string {
	return `WITH RECURSIVE member_identities(id) AS (` + roots + `
 UNION SELECT retired.id FROM app_user retired JOIN member_identities survivor
 ON retired.merged_into_user_id = survivor.id WHERE retired.removed_at IS NOT NULL)
 SELECT id FROM member_identities`
}

func memberAuthorshipSQL(alias string, memberPosition int, arg func(any) int) string {
	return fmt.Sprintf(`EXISTS (SELECT 1 FROM (%s) author_identity WHERE %s.captured_by LIKE $%d || author_identity.id::text)`, memberIdentitySQL(memberPosition), alias, arg("%:"))
}
