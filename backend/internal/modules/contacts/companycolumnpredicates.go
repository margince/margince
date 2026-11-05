// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import "strings"

// Previews share the writer predicates without carrying executable UPDATE statements.
var companyColumnReadPredicates = func() map[string]map[companyWriteAuthority]string {
	predicates := make(map[string]map[companyWriteAuthority]string, len(companyColumnWrites))
	for column, write := range companyColumnWrites {
		predicates[column] = make(map[companyWriteAuthority]string)
		for authority, statement := range write {
			_, predicate, found := strings.Cut(statement, " WHERE ")
			if found {
				predicates[column][authority] = predicate
			}
		}
	}
	return predicates
}()
