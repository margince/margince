// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H2
package gates

import (
	"os"
	"sort"
	"strings"
	"testing"
)

// A touch trigger may only sit on a table that has every column it writes.
//
// set_updated_at_bump_version assigns NEW.version, so on a table without a
// version column every UPDATE fails with `record "new" has no field
// "version"`. Nothing notices while the table is only ever inserted into: the
// first UPDATE is the first failure, and a data sweep migration that meets a
// populated table is where it surfaces — on an operator's database, never on
// CI's empty one.
func TestEveryTouchTriggerTableHasTheColumnsItsTriggerWrites(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("migrations/testdata/head_catalog.txt")
	if err != nil {
		t.Fatalf("reading the head catalog: %v", err)
	}
	// A column line is `public.<table>.<column> <type> … gen=…`; constraint,
	// index and trigger lines share the prefix but never carry gen=.
	columns := map[string]bool{}
	for line := range strings.SplitSeq(string(raw), "\n") {
		name, rest, found := strings.Cut(strings.TrimSpace(line), " ")
		if found && strings.HasPrefix(name, "public.") && strings.Contains(rest, " gen=") {
			columns[name] = true
		}
	}
	var missing []string
	for table, written := range touchTriggerTables(t) {
		for column := range written {
			if !columns["public."+table+"."+column] {
				missing = append(missing, table+"."+column)
			}
		}
	}
	sort.Strings(missing)
	for _, column := range missing {
		t.Errorf("%s: the table's touch trigger writes this column and the table has none — "+
			"every UPDATE of it fails; point the trigger at the function whose columns the table has", column)
	}
}
