// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// array_length answers NULL for an empty array rather than 0, so a CHECK that
// bounds a length with it evaluates to UNKNOWN for `{}`, and Postgres accepts a
// row on an UNKNOWN check. A constraint written to require at least one element
// therefore admits the one value it exists to refuse. array_upper, array_lower
// and array_ndims answer NULL for `{}` too and carry the identical hole.
//
// cardinality() answers 0 and has no such hole; coalesce(array_length(c,1),0)
// closes it explicitly. Both spellings are already in this tree, which is why
// this reads the built schema rather than the migration text: the catalog is
// what the database ENDS UP with, so a constraint re-added by a later
// migration is judged in its final form and an older file's wording cannot
// fail a column that was since repaired.
//
// The corpus is the CORE catalog and nothing else. A per-workspace custom
// column, an extension's own migrations and DDL a module generates at runtime
// never reach this file, so none of them is held here — and an empty array is
// the right answer for some of them, an unfilled multi-select among them.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// bareArrayBound finds a length-ish array function that is not already wrapped
// in a coalesce. jsonb_array_length is excluded by the leading boundary: it
// answers 0 for an empty JSON array and carries no NULL hole.
var bareArrayBound = regexp.MustCompile(
	`(?:^|[^_[:alnum:]])(array_(?:length|upper|lower|ndims))\s*\(`)

// catalogEntryStart marks the first line of a catalog entry. A definition that
// wraps — a CASE inside a CHECK does — continues on lines that match nothing
// here, and scanning those pieces separately would read a bare call as a line
// carrying no CHECK at all: a census that fails short reports PASS.
var catalogEntryStart = regexp.MustCompile(`^(?:public\.|defacl |schema )`)

func TestNoCheckBoundsAnArrayLengthThatCanBeNull(t *testing.T) {
	t.Parallel()
	path := filepath.Join(repoRoot, "backend", "migrations", "testdata", "head_catalog.txt")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	var offenders []string
	for _, entry := range catalogEntries(string(raw)) {
		if !strings.Contains(entry, "CHECK") || !hasBareArrayBound(entry) {
			continue
		}
		offenders = append(offenders, strings.TrimSpace(entry))
	}
	if len(offenders) == 0 {
		return
	}
	t.Errorf("a CHECK bounds an array with array_length, array_upper, array_lower or "+
		"array_ndims, each of which is NULL for `{}` and makes the constraint UNKNOWN — "+
		"Postgres accepts the empty array the constraint exists to refuse. Write "+
		"cardinality(c) or coalesce(array_length(c, 1), 0) in a new migration:\n  %s",
		strings.Join(offenders, "\n  "))
}

// catalogEntries folds each entry's continuation lines back onto it. Joining
// with a space rather than a newline keeps a wrapped expression readable as
// one statement, which is how the scan below reads it.
func catalogEntries(raw string) []string {
	var entries []string
	for line := range strings.SplitSeq(raw, "\n") {
		if len(entries) == 0 || catalogEntryStart.MatchString(line) {
			entries = append(entries, line)
			continue
		}
		entries[len(entries)-1] += " " + line
	}
	return entries
}

// hasBareArrayBound reports whether any such call in the entry is not the
// argument of a coalesce. Scanning per call rather than per entry matters: a
// CHECK with one guarded and one bare call is an offender, and an entry-level
// `strings.Contains(entry, "coalesce")` would clear it.
func hasBareArrayBound(entry string) bool {
	lower := strings.ToLower(entry)
	for _, at := range bareArrayBound.FindAllStringSubmatchIndex(lower, -1) {
		prefix := strings.TrimRight(lower[:at[2]], " (")
		if strings.HasSuffix(prefix, "coalesce") {
			continue
		}
		return true
	}
	return false
}
