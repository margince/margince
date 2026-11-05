// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"strings"
	"testing"
)

func TestCompanyReadPredicatesCoverEveryColumnWriter(t *testing.T) {
	if len(companyColumnReadPredicates) != len(companyColumnWrites) || len(companyColumnWrites) == 0 {
		t.Fatal("preview predicates must cover the column writers")
	}
	for column, write := range companyColumnWrites {
		for authority, statement := range write {
			predicate := companyColumnReadPredicates[column][authority]
			if predicate == "" || !strings.HasSuffix(statement, " WHERE "+predicate) {
				t.Errorf("%s authority %d: preview does not use the writer predicate", column, authority)
			}
		}
	}
}
