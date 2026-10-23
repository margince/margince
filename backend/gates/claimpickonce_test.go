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

// claimSubqueryBody is the text of a subquery, with parentheses nested two deep and no
// further. A body that cannot cross its own closing parenthesis is what keeps one claim's
// `IN (SELECT id FROM picked)` from reaching the next claim's lock clause in the same file.
const claimSubqueryBody = `(?:[^()]|\((?:[^()]|\([^()]*\))*\))*?`

// claimedByInSubquery matches an `IN` or `= ANY` subquery that locks the rows it picks, in
// any spelling of the lock clause.
var claimedByInSubquery = regexp.MustCompile(`(?is)(?:\bIN|=\s*ANY)\s*\(\s*SELECT` + claimSubqueryBody +
	`FOR\s+(?:NO\s+KEY\s+)?UPDATE(?:\s+OF\s+[\w\s,.]+?)?\s+SKIP\s+LOCKED\s*\)`)

// skipLockedSite matches one use of the lock clause, so the walk can tell it read the sites.
var skipLockedSite = regexp.MustCompile(`(?i)SKIP\s+LOCKED`)

// claimPickFloor is how many hand-written Go files the walk must read, and claimSitesFloor how
// many uses of SKIP LOCKED it must see among them. A walk that reads none would report a
// clean tree from nothing.
const (
	claimPickFloor  = 1500
	claimSitesFloor = 28
)

func TestNoClaimQueryPicksItsRowsInAnInSubquery(t *testing.T) {
	t.Parallel()

	read, sites := 0, 0
	for _, root := range []string{"internal", "cmd", "pkg", "../extensions"} {
		walkGoFiles(t, root, func(rel string, body []byte) {
			read++
			sites += len(skipLockedSite.FindAll(body, -1))
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
	if sites < claimSitesFloor {
		t.Fatalf("saw only %d use(s) of SKIP LOCKED and expects at least %d — the walk misses claims", sites, claimSitesFloor)
	}
}

func TestTheClaimPickPatternSeesEveryShapeItForbids(t *testing.T) {
	t.Parallel()

	for name, query := range map[string]string{
		"plain lock clause":          "UPDATE t SET a = 1 WHERE t.id IN (\n SELECT id FROM t WHERE due\n ORDER BY due LIMIT $1\n FOR UPDATE SKIP LOCKED)",
		"lock clause naming a table": "UPDATE t SET a = 1 WHERE t.id IN (SELECT id FROM t LIMIT $1 FOR UPDATE OF t SKIP LOCKED)",
		"no key update":              "UPDATE t SET a = 1 WHERE t.id IN (SELECT id FROM t LIMIT $1 FOR NO KEY UPDATE SKIP LOCKED)",
		"= ANY subquery":             "UPDATE t SET a = 1 WHERE t.id = ANY (SELECT id FROM t LIMIT $1 FOR UPDATE SKIP LOCKED)",
		"nested parentheses":         "UPDATE t SET a = 1 WHERE t.id IN (SELECT id FROM t WHERE (due IS NULL OR due <= now()) LIMIT $1 FOR UPDATE SKIP LOCKED)",
	} {
		if !claimedByInSubquery.MatchString(query) {
			t.Errorf("the pattern misses an UPDATE that picks its rows in an IN subquery: %s", name)
		}
	}
}

func TestTheClaimPickPatternLeavesAClaimThatPicksOnce(t *testing.T) {
	t.Parallel()

	for name, query := range map[string]string{
		"one claim": "WITH picked AS MATERIALIZED (SELECT id FROM t LIMIT $1 FOR UPDATE SKIP LOCKED)\n" +
			"UPDATE t SET a = 1 WHERE t.id IN (SELECT id FROM picked)",
		"two claims in one file": "WITH picked AS MATERIALIZED (SELECT id FROM t LIMIT $1 FOR UPDATE SKIP LOCKED)\n" +
			"UPDATE t SET a = 1 WHERE t.id IN (SELECT id FROM picked)\n" +
			"WITH picked AS MATERIALIZED (SELECT id FROM u LIMIT $1 FOR UPDATE SKIP LOCKED)\n" +
			"UPDATE u SET a = 1 WHERE u.id IN (SELECT id FROM picked)",
		"a lock clause outside an IN": "SELECT id FROM t WHERE due <= now() LIMIT $1 FOR UPDATE SKIP LOCKED",
	} {
		if claimedByInSubquery.MatchString(query) {
			t.Errorf("the pattern flags a claim that picks its rows once: %s", name)
		}
	}
}
