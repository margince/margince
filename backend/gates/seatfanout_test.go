// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// A nightly agent acts FOR A PERSON, and the identity of one night's work has
// to say which person. This gate holds the half that fails silently.
//
// Two uniqueness rules stop a scheduled occurrence running twice:
// agent_run_trigger_unique on trigger_ref alone, and runner_job_trigger_unique
// on (agent_spec, trigger_ref). Both are keyed on the trigger ref, so what that
// string distinguishes is exactly what the database will let run in parallel. A
// ref of `<spec>:<date>` therefore makes the whole night workspace-wide: the
// first seat seeded takes the row and every other rep's insert conflicts away.
//
// NOTHING ERRORS WHEN THAT HAPPENS. One row inserting and the rest conflicting
// is byte-for-byte what a correct re-seed looks like — ON CONFLICT DO NOTHING
// is the intended path, the tick returns nil, and the log is quiet. The team
// simply does not get briefs, and the first person to notice is a rep who
// wonders where theirs went. That is why this is a gate and not a test: the
// defect's signature is the absence of work, and absence is what a passing
// suite looks like.
//
// So the obligation is on the CALL: every production caller of TriggerRef
// passes a seat. The signature already forces an argument, but a signature
// cannot stop the next author threading a constant, a zero value, or one seat
// resolved outside the loop through it — which compiles, reads fine, and
// restores the workspace-wide behaviour exactly.
//
// The obligation belongs to the SEEDING path, which is the only place a ref
// becomes a row. A caller that mints a ref to read its SHAPE and throws the
// value away carries no fan-out question at all, and there is one: the
// certification fixture validator derives what a corpus trigger ref must look
// like by asking the writer, rather than restating the format a second time.
// It is named below with its reason. What would make that waiver wrong is the
// value reaching a job — so the reason says what it is used for, and a reader
// who finds it doing anything else should delete the entry rather than widen
// it.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// The two constraints whose key IS the trigger ref. Named here because the
// gate's whole argument rests on them: if a later migration re-keys either one
// to include a user column, the ref no longer has to carry the seat and this
// gate is describing a rule that moved.
var triggerKeyedConstraints = []string{
	"agent_run_trigger_unique",
	"runner_job_trigger_unique",
}

// TestEveryScheduledOccurrenceIsNamedForOneSeat holds the seeding half: no
// production call may build a trigger ref without a per-seat value.
//
// It checks that the argument is a SELECTOR on a range variable — the
// `grant.UserID` shape — rather than merely non-empty. A constant, a captured
// variable from outside the loop, or a zero id all satisfy "an argument was
// passed" while seeding one identical ref for every seat, which is the bug
// this exists to prevent.
func TestEveryScheduledOccurrenceIsNamedForOneSeat(t *testing.T) {
	t.Parallel()
	calls := triggerRefCalls(t)
	if len(calls) == 0 {
		t.Fatal("no production call to TriggerRef was found: this gate proves nothing " +
			"unless it can see the seeding path, so either the scan broke or the " +
			"caller moved and this gate must move with it")
	}
	for _, c := range calls {
		if mintsAShapeRatherThanAnOccurrence.Waived(t, c.site) {
			continue
		}
		if c.seatArg == "" {
			t.Errorf("%s: TriggerRef is called without a seat, so every rep in the "+
				"workspace shares one trigger ref and only the first seeded gets a run", c.pos)
			continue
		}
		if !c.fromRangeVar {
			t.Errorf("%s: TriggerRef is called with %q, which does not resolve to the "+
				"variable of the loop it sits in — a ref built from a constant, a captured "+
				"id, or one seat resolved outside the loop is the SAME ref for every rep, "+
				"and the uniqueness constraints then admit exactly one run for the whole "+
				"workspace", c.pos, c.seatArg)
		}
	}
	mintsAShapeRatherThanAnOccurrence.AssertAllMatched(t)
}

// mintsAShapeRatherThanAnOccurrence names the calls that build a reference
// value to COMPARE against, never one to seed with. A seat that varies per
// iteration is meaningless to them: nothing they produce is inserted, so
// nothing they produce can collide.
var mintsAShapeRatherThanAnOccurrence = gatekit.Waive(map[string]string{
	"internal/compose/certcase_agentlooptrigger.go:mintedSchedulerTriggerRef": "mints a ref from a fixed day and seat so the certification fixture validator can compare a corpus trigger ref's SHAPE against what the writer produces, rather than restating the format; the value is compared and discarded, and never reaches EnqueueJob or a job row",
})

// TestTheTriggerRefStillCarriesTheWholeUniquenessKey fails when a migration
// re-keys either constraint away from the trigger ref.
//
// The gate above is only correct while these two constraints are what stops a
// double run. A migration that adds a user column to either key makes the seat
// segment redundant rather than load-bearing, and a later author reading only
// the gate above would be told to preserve a rule that no longer holds.
func TestTheTriggerRefStillCarriesTheWholeUniquenessKey(t *testing.T) {
	t.Parallel()
	sql := readMigrations(t)
	for _, name := range triggerKeyedConstraints {
		// The LAST mention, not the first: the baseline defines these
		// constraints, so reading the first occurrence would keep answering from
		// the baseline even after a later migration dropped or re-keyed one —
		// the gate would stay green about a rule that no longer exists.
		idx := strings.LastIndex(sql, name)
		if idx < 0 {
			t.Errorf("constraint %s is gone from the migrations: the seat segment in "+
				"TriggerRef exists to make THIS key per-seat, so if the key moved, "+
				"TestEveryScheduledOccurrenceIsNamedForOneSeat is now guarding a rule "+
				"that no longer decides anything", name)
			continue
		}
		clause := sql[idx:]
		if end := strings.Index(clause, ";"); end >= 0 {
			clause = clause[:end]
		}
		if strings.Contains(clause, "user_id") || strings.Contains(clause, "passport_id") {
			t.Errorf("constraint %s now keys on a seat column directly: %s\n"+
				"the trigger ref no longer has to carry the seat, so revisit "+
				"TriggerRef and this gate together", name, strings.TrimSpace(clause))
		}
	}
}

// triggerRefCall is one production call site and the expression it passes as
// the seat.
type triggerRefCall struct {
	pos     string
	site    string
	seatArg string
	// fromRangeVar is whether the seat traces to the range variable of a loop
	// enclosing the call. That is the property the fan-out actually needs: a
	// value that varies per iteration. "an argument was passed" and "it has a
	// dot in it" are both satisfied by a seat resolved ONCE outside the loop,
	// which compiles, reads correctly, and seeds one identical ref for the
	// whole workspace.
	fromRangeVar bool
}

// triggerRefCalls parses the non-test tree and returns every call whose
// selector is TriggerRef.
//
// It reads the syntax tree rather than the file text: a text scan counts the
// name inside a comment or a string literal, so a gate built that way stays
// green while the real call loses its argument. Under-recognition is the one
// way a census must not break.
func triggerRefCalls(t *testing.T) []triggerRefCall {
	t.Helper()
	var out []triggerRefCall
	fset := token.NewFileSet()
	const root = "internal"
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, perr := parser.ParseFile(fset, path, nil, 0)
		if perr != nil {
			return perr
		}
		// The range variables currently in scope, innermost last. A call is
		// only per-seat if its seat argument is rooted in one of them.
		var inScope []string
		// The function a call sits in, so a waiver names one call site rather
		// than a whole file. A call outside any function declaration keeps the
		// empty name, which matches no waiver and so must satisfy the seat rule
		// on its own — the safe direction for a reader that meets a shape it
		// was not built for.
		enclosing := ""
		var walk func(ast.Node) bool
		walk = func(n ast.Node) bool {
			if fn, ok := n.(*ast.FuncDecl); ok {
				if fn.Body == nil {
					return false
				}
				before := enclosing
				enclosing = fn.Name.Name
				ast.Inspect(fn.Body, walk)
				enclosing = before
				return false
			}
			if rng, ok := n.(*ast.RangeStmt); ok {
				before := len(inScope)
				for _, key := range []ast.Expr{rng.Key, rng.Value} {
					if id, ok := key.(*ast.Ident); ok && id.Name != "_" {
						inScope = append(inScope, id.Name)
					}
				}
				ast.Inspect(rng.Body, walk)
				inScope = inScope[:before]
				return false
			}
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "TriggerRef" {
				return true
			}
			c := triggerRefCall{pos: fset.Position(call.Pos()).String(), site: path + ":" + enclosing}
			// The seat is the LAST argument: the day names the occurrence and
			// the seat names whose occurrence it is.
			if n := len(call.Args); n >= 2 {
				c.seatArg = exprText(fset, call.Args[n-1])
				c.fromRangeVar = rootedIn(call.Args[n-1], inScope)
			}
			out = append(out, c)
			return true
		}
		ast.Inspect(file, walk)
		return nil
	})
	if err != nil {
		t.Fatalf("scanning for TriggerRef callers: %v", err)
	}
	return out
}

// rootedIn reports whether an expression bottoms out in one of the named
// variables: `grant`, `grant.UserID`, `grant.Seat.ID` all count for "grant".
func rootedIn(expr ast.Expr, names []string) bool {
	for {
		switch e := expr.(type) {
		case *ast.SelectorExpr:
			expr = e.X
		case *ast.Ident:
			for _, name := range names {
				if e.Name == name {
					return true
				}
			}
			return false
		default:
			return false
		}
	}
}

// readMigrations concatenates the core migrations so a constraint can be read
// as it will actually exist.
func readMigrations(t *testing.T) string {
	t.Helper()
	var b strings.Builder
	dir := filepath.Join("migrations", "core")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading the core migrations: %v", err)
	}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".up.sql") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("reading %s: %v", e.Name(), err)
		}
		b.Write(raw)
		b.WriteString("\n")
	}
	return b.String()
}
