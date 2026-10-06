// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

import (
	"regexp"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/modules/contacts"
)

// The mark reference check reads every column a mark's key can be stored in.
//
// It decides what the stored-object sweep deletes, so a column it does not read is a
// live logo reported as referenced by nobody — the bytes go, and the record shows a
// broken image.
//
// The QUERY is built from contacts.LogoKeyColumns, so that half needs no gate. What
// no code there can check is the list against the schema, which is this: a new mark
// column is simply not looked at, and nothing in Go says it exists.
func TestTheMarkReferenceCheckReadsEveryMarkColumn(t *testing.T) {
	t.Parallel()
	declared := map[string]bool{}
	for table, columns := range contacts.LogoKeyColumns {
		for _, column := range columns {
			declared[table+"."+column] = true
		}
	}

	found := markColumnsInTheSchema(t)
	// Under-recognition is the one way this gate must not break: a catalog read that
	// matches nothing agrees with any list at all and asserts nothing.
	if len(found) < 4 {
		t.Fatalf("read %d mark columns from the committed catalog, fewer than the four this gate "+
			"was written against: the pattern has drifted from the schema", len(found))
	}
	for _, column := range found {
		if !declared[column] {
			t.Errorf("%s can hold a mark's key and contacts.LogoKeyColumns does not name it, so the "+
				"sweep reads a live mark as unreferenced and deletes the bytes behind it", column)
		}
	}
	for column := range declared {
		if !slices.Contains(found, column) {
			t.Errorf("contacts.LogoKeyColumns names %s, which the schema has no such column for — "+
				"the check queries a column that is not there", column)
		}
	}
}

// markColumnsInTheSchema reads the committed catalog for every column that holds a
// mark's object key, derived rather than listed so a new one joins this gate's
// corpus by existing.
func markColumnsInTheSchema(t *testing.T) []string {
	t.Helper()
	catalog := readFile(t, moduleRoot(t)+"/migrations/testdata/head_catalog.txt")
	pattern := regexp.MustCompile(`(?m)^public\.(\w+)\.(logo\w*_object_key) text`)
	var found []string
	for _, m := range pattern.FindAllStringSubmatch(catalog, -1) {
		found = append(found, m[1]+"."+m[2])
	}
	return found
}
