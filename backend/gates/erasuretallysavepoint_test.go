// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind reachability H2

package gates

import (
	"os"
	"regexp"
	"sort"
	"testing"
)

// The erasure counts its work on a tallyTx (privacy/erasuretally.go), and a
// savepoint opened on it runs on the wrapped transaction, where nothing counts.
// So no file the cascade reaches opens one. Otherwise a request's proof would
// leave that arm's rows out and still read as complete.
var savepointOpen = regexp.MustCompile(`\.Begin\(ctx\)|storekit\.Savepoint\(`)

func TestTheErasureCascadeOpensNoSavepoint(t *testing.T) {
	t.Parallel()
	reached := filesReachableFrom(t, "internal/modules/privacy", "EraseContact")
	if len(reached) < 10 {
		t.Fatalf("the walk reached %d file(s), want at least ten — it is not reading the cascade", len(reached))
	}
	var opening []string
	for path := range reached {
		source, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}
		if savepointOpen.Match(source) {
			opening = append(opening, path)
		}
	}
	sort.Strings(opening)
	if len(opening) > 0 {
		t.Errorf("the erasure cascade opens a savepoint in %v; its statements would escape the tally. "+
			"Count them on the tallyTx instead, or teach tallyTx to count a savepoint", opening)
	}
}
