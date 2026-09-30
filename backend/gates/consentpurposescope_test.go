// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build !integration

//gate:kind claim H1

package gates

// consent_purpose is installation-wide configuration, not a row-scoped record.
//
// requireConfirmablePurposeTx tells an ARCHIVED purpose (422) apart from an
// absent one (404), which discloses nothing only while there is no scope a
// caller can be outside. The day the table grows one, that 422 becomes a
// disclosure and the door's doc comment becomes an argument for it — the shape
// of comment that survives review because it reads as settled.
//
// One table, because the claim is about one door's answer: a census of every
// table read without a scope clause would have to recognise a scope clause in
// the AST to be honest about what it skipped.

import (
	"slices"
	"testing"
)

// scopeColumns are what auth reads to bound a row to a caller: ownerPredicate
// takes owner_id, and the capture-privacy arm pairs it with visibility.
var scopeColumns = []string{"owner_id", "visibility"}

func TestAConsentPurposeIsNotARowScopedRecord(t *testing.T) {
	t.Parallel()
	schema := parseHeadCatalog(t)
	table, found := schema.tables["consent_purpose"]
	if !found {
		t.Fatal("consent_purpose is gone from the head catalog: the door whose 404/422 split this " +
			"exempts reads that table, so either the table was renamed and this gate did not follow, " +
			"or the catalog no longer reaches it and this gate is holding nothing")
	}
	for _, column := range table.columns {
		if slices.Contains(scopeColumns, column.name) {
			t.Errorf("consent_purpose now carries %q, so a caller can be OUTSIDE a purpose — and "+
				"requireConfirmablePurposeTx's 422 for an archived one tells such a caller it exists. "+
				"Either answer 404 for archived as well (and update the door's doc comment and "+
				"consent/doidoor_integration_test.go), or state here why the column does not scope it.",
				column.name)
		}
	}
}
