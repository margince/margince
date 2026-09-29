// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// array_length answers NULL for an EMPTY array, not 0, so a CHECK that bounds
// a length with it evaluates to UNKNOWN for `{}` — and Postgres ACCEPTS a row
// on an UNKNOWN check. A constraint written to require at least one element
// therefore admits the one value it exists to refuse.
//
// cardinality() answers 0 and has no such hole; coalesce(array_length(c,1),0)
// closes it explicitly. Both spellings are already in this tree, which is why
// this reads the built schema rather than the migration text: the catalog is
// what the database ENDS UP with, so a constraint re-added by a later
// migration is judged in its final form and an older file's wording cannot
// fail a column that was since repaired.

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

const emptyArrayCheckCatalog = "migrations/testdata/head_catalog.txt"

// bareArrayLength finds array_length( that is not already wrapped in a
// coalesce. jsonb_array_length is excluded by the leading boundary: it answers
// 0 for an empty JSON array and carries no NULL hole.
var bareArrayLength = regexp.MustCompile(`(?:^|[^_[:alnum:]])(array_length)\s*\(`)

func TestNoCheckBoundsAnArrayLengthThatCanBeNull(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile(emptyArrayCheckCatalog)
	if err != nil {
		t.Fatalf("reading %s: %v", emptyArrayCheckCatalog, err)
	}
	var offenders []string
	for _, line := range strings.Split(string(raw), "\n") {
		if !strings.Contains(line, "CHECK") || !bareArrayLength.MatchString(strings.ToLower(line)) {
			continue
		}
		// A guarded call still matches the expression above; the guard is what
		// makes it safe, so the line is only an offender when some call is bare.
		if !hasBareArrayLength(line) {
			continue
		}
		offenders = append(offenders, strings.TrimSpace(line))
	}
	if len(offenders) == 0 {
		return
	}
	t.Errorf("a CHECK bounds an array length with array_length, which is NULL for "+
		"`{}` and makes the constraint UNKNOWN — Postgres accepts the empty array "+
		"the constraint exists to refuse. Write cardinality(c) or "+
		"coalesce(array_length(c, 1), 0) in a new migration:\n  %s",
		strings.Join(offenders, "\n  "))
}

// hasBareArrayLength reports whether any array_length call on the line is not
// the argument of a coalesce. Scanning per call rather than per line matters:
// a CHECK with one guarded and one bare call is an offender, and a line-level
// `strings.Contains(line, "coalesce")` would clear it.
func hasBareArrayLength(line string) bool {
	lower := strings.ToLower(line)
	for _, at := range bareArrayLength.FindAllStringSubmatchIndex(lower, -1) {
		prefix := strings.TrimRight(lower[:at[2]], " (")
		if strings.HasSuffix(prefix, "coalesce") {
			continue
		}
		return true
	}
	return false
}
