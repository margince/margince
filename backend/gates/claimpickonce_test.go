// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H2

//go:build !integration

package gates

// A claim query picks its rows once, in a MATERIALIZED CTE, and never in an IN subquery.
//
// An UPDATE that takes rows through `id IN (SELECT ... LIMIT n FOR UPDATE SKIP LOCKED)` leaves
// the planner free to run the subquery again for each row it visits. Each run skips the rows
// the statement already took and finds n more, so one statement claims more than its limit. A
// pass that bounds its work by that limit then does unbounded work, and only on a plan the
// table's statistics choose.

import (
	"regexp"
	"testing"
)

// claimedByInSubquery matches an IN subquery that locks the rows it picks.
var claimedByInSubquery = regexp.MustCompile(`(?is)\bIN\s*\(\s*SELECT[^;]*?FOR\s+UPDATE\s+SKIP\s+LOCKED\s*\)`)

// claimPickFloor is how many hand-written Go files the walk must read. A walk that reads none
// would report a clean tree from nothing.
const claimPickFloor = 1500

func TestNoClaimQueryPicksItsRowsInAnInSubquery(t *testing.T) {
	t.Parallel()

	read := 0
	for _, root := range []string{"internal", "cmd", "pkg"} {
		walkGoFiles(t, root, func(rel string, body []byte) {
			read++
			if claimedByInSubquery.Match(body) {
				t.Errorf("%s claims rows through an IN subquery with FOR UPDATE SKIP LOCKED. The planner can "+
					"run it again per row and claim more than its LIMIT; pick the rows in a "+
					"`WITH picked AS MATERIALIZED (...)` and update `WHERE id IN (SELECT id FROM picked)`", rel)
			}
		})
	}
	if read < claimPickFloor {
		t.Fatalf("read only %d Go file(s) and expects at least %d — the walk is broken, not the tree", read, claimPickFloor)
	}
}

func TestTheClaimPickPatternSeesTheShapeItForbids(t *testing.T) {
	t.Parallel()

	forbidden := "UPDATE t SET a = 1 WHERE t.id IN (\n SELECT id FROM t WHERE due\n ORDER BY due LIMIT $1\n FOR UPDATE SKIP LOCKED)"
	allowed := "WITH picked AS MATERIALIZED (SELECT id FROM t LIMIT $1 FOR UPDATE SKIP LOCKED)\n" +
		"UPDATE t SET a = 1 WHERE t.id IN (SELECT id FROM picked)"
	if !claimedByInSubquery.MatchString(forbidden) {
		t.Error("the pattern misses an UPDATE that picks its rows in an IN subquery")
	}
	if claimedByInSubquery.MatchString(allowed) {
		t.Error("the pattern flags a claim that picks its rows once in a MATERIALIZED CTE")
	}
}
