// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H1

package gates

// A fixture that needs THIS SESSION'S seat does not pick one out of app_user.
//
// `SELECT id FROM app_user WHERE is_agent = false ORDER BY created_at LIMIT 1`
// names the seat a scenario signed in as only while the installation holds
// exactly one contact. Rows inserted in one transaction share `now()`, so with a
// second contact the tie-break is whatever the plan returns — and the pick
// silently becomes somebody else.
//
// It fails in the worst direction available, which is why this is a gate and
// not a preference. demoteToRep demoted a seat the session was not using, the
// session stayed admin, and a test asserting an admin-only route answers 403
// got a 200 — reported as a permission defect in the route, in a full parallel
// lane, green in isolation and green on the next run. The same pick then turned
// up twice more: staging an approval inbox on behalf of an arbitrary seat, and
// recording a directed-send decision as one.
//
// Three copies of one mistake is what makes it a rule rather than three fixes.
//
// WHAT COUNTS AS PINNED. A statement that names the row — `WHERE id = $1`,
// `WHERE email = $1` — is answering a question the caller already settled, and
// is not this defect. So is one with no `LIMIT`: reading every seat is a
// different question from picking one. What is refused is a statement that
// takes ONE row out of app_user on an ordering that does not decide which.
//
// THE FIX IS `sessionUserID`, in compose/integration: it reads the signed-in
// seat from `GET /v1/me`, which is the same resolution the handlers take. A
// fixture deriving production's answer for itself is free to derive a different
// one, and here "different" means acting as a stranger.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// picksOneSeat matches a read of app_user bounded to a single row.
//
// `(?s)` so the pattern spans the newlines a formatted statement carries, and
// the LIMIT is looked for anywhere after the table because a real statement puts
// a WHERE, an ORDER BY or a join between them.
var picksOneSeat = regexp.MustCompile(`(?is)\bfrom\s+app_user\b.*\blimit\s+1\b`)

// pinsTheRow matches the predicates that decide WHICH seat, so a statement
// carrying one is answering a question its caller already settled.
//
// TWO SHAPES, because the tree names a seat two ways. A bound id or email is a
// caller naming the row. A correlated COMPARISON against the joined row —
// `a.captured_by = 'human:' || u.id::text` inside a LATERAL, and the LIKE form
// beside it — names it from the row being joined, which decides the pick
// exactly as a bind would; the `LIMIT 1` there bounds a union's classes rather
// than choosing between seats.
//
// The comparison is matched and not the expression: `u.id::text` alone appears
// in a PROJECTION too, and `SELECT u.id::text FROM app_user u LIMIT 1` is this
// defect wearing the shape that excuses it. The operator and the prefix are
// left open — `=` or `LIKE`, any literal — because what makes it a pin is being
// compared to a column of the other row, not which prefix the tree happens to
// use today.
//
// `id <> $1` is deliberately NOT one of them. It says which seat the pick is
// not, and leaves the ordering to choose among the rest — which is this defect
// with one row excluded from it. A statement meaning "the colleague" says so in
// a waiver, because "any of the others" is a claim about the fixture and not
// about the SQL.
var pinsTheRow = regexp.MustCompile(
	`(?is)\b(id|email)\s*=\s*\$\d` +
		`|\b[a-z_][a-z0-9_]*\.[a-z_][a-z0-9_]*\s*(=|like)\s*'[^']*'\s*\|\|\s*u\.id::text\b`)

// seatPickWaivers ratifies a single-row pick that is NOT resolving the session's
// own seat. Each says which seat it means instead.
var seatPickWaivers = gatekit.Waive(map[string]string{
	"internal/compose/installation.go":                                   "seedBookingPage runs inside the INSTALLATION seed, where exactly one non-agent seat exists — the bootstrap admin it is provisioning the page for. Its own comment already names the hazard this gate is about (\"first by created_at is heap order between two rows written in one transaction\"); what makes it safe is that there is no second row yet, not the ordering",
	"internal/modules/privacy/sarmergelineage_integration_test.go":       "seedRefusalAgainst needs A user to hang a transmit-refusal decision off, and the decision is about the SUBJECT rather than about whoever recorded it — the fixture asserts what the subject-access export reaches, and the recorder never appears in it. Any seat will do, and no seat is the session's",
	"internal/compose/integration/fieldhistory_http_integration_test.go": "seedHumanFieldHistoryRow wants a REAL seat rather than a particular one, which its own comment states: the name resolution joins app_user on the `human:` || id key, so a made-up id would exercise the join against a shape no write produces. Which real seat is immaterial — the assertion is that a name resolves at all",
	"internal/compose/captureofflinedemo.go":                             "the colleague to CC in the offline capture demo, and its own comment says what it wants: \"any other seat will do — the point is that a thread sometimes has a third party, not which one\". It is a development seeder, and `id <> $1` bounds the pick away from the session's own seat — which says which seat it is NOT, and is why this is a ratification rather than a pattern",
})

// wantSeatPickFloor is what the walk judged when this gate landed. A pattern
// that stops matching app_user finds nothing to object to and reads exactly
// like a clean tree.
const wantSeatPickFloor = 20

func TestNoFixturePicksASeatAnOrderingDoesNotDecide(t *testing.T) {
	t.Parallel()
	defer seatPickWaivers.AssertAllMatched(t)

	fset := token.NewFileSet()
	judged := 0
	for _, path := range handWrittenGoSources(t) {
		slash := filepath.ToSlash(path)
		// This file plants the defect below as the gate's own evidence.
		if filepath.Base(path) == "seatfixturepick_test.go" {
			continue
		}
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", path, err)
		}
		for _, sql := range appUserSQL(file) {
			if !strings.Contains(strings.ToLower(sql), "app_user") {
				continue
			}
			judged++
			if !picksOneSeat.MatchString(sql) || pinsTheRow.MatchString(sql) {
				continue
			}
			if seatPickWaivers.Waived(t, slash) {
				continue
			}
			t.Errorf("%s takes ONE row out of app_user on an ordering that does not decide which:\n\t%s\n\n"+
				"Rows inserted in one transaction share now(), so a second contact makes this pick "+
				"arbitrary — and a fixture acting as a stranger fails as a defect in whatever it was "+
				"asserting about. Read the signed-in seat instead (sessionUserID, compose/integration), "+
				"or pin the row by id or email.",
				slash, gatekit.FirstLineOf(sql))
		}
	}
	if judged < wantSeatPickFloor {
		t.Errorf("the walk judged %d app_user statement(s) and judged at least %d when this gate "+
			"landed — it has stopped recognising this tree's SQL, which reads the same as a clean tree",
			judged, wantSeatPickFloor)
	}
}

// appUserSQL collects the flattened SQL literals in one file that name app_user.
func appUserSQL(file *ast.File) []string {
	var out []string
	for _, text := range gatekit.SQLStatementsOf(file) {
		if strings.Contains(strings.ToLower(text), "app_user") {
			out = append(out, text)
		}
	}
	return out
}
