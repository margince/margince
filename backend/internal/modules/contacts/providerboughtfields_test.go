// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"strings"
	"testing"
)

// TestTheRevertAndTheMarksShareOnePredicatePerTable fails when either side
// restates "is this still the purchase's" instead of reading boughtPredicates,
// so the page cannot mark a value the revert would treat as somebody else's.
func TestTheRevertAndTheMarksShareOnePredicatePerTable(t *testing.T) {
	for _, table := range []string{
		entityContact, tableContactSocial, tableContactEmail, tableContactPhone, tableRelationship,
	} {
		predicate, ok := boughtPredicates[table]
		if !ok {
			t.Errorf("a purchase can fill %s and no predicate says when it still owns it", table)
			continue
		}
		statement, ok := revertStatements[table]
		if !ok {
			t.Errorf("a purchase can fill %s and the revert has no statement for it", table)
			continue
		}
		if !strings.Contains(statement, predicate) {
			t.Errorf("the %s revert no longer builds on the shared predicate", table)
		}
		if !readTests(table, predicate) {
			t.Errorf("the bought-fields read does not test %s with the shared predicate", table)
		}
	}
}

// readTests reports whether the read's arm over table opens its WHERE with the
// predicate.
func readTests(table, predicate string) bool {
	_, arm, found := strings.Cut(boughtFieldsSQL, "FROM "+table+" t")
	if !found {
		return false
	}
	_, where, found := strings.Cut(arm, "WHERE ")
	return found && strings.HasPrefix(where, predicate)
}
