// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

import (
	"go/ast"
	"go/parser"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// A seat's fitness to receive work has one spelling, wherever it is asked.
//
// eligibilityColumns are the user columns that decide it. A second reader of
// this SET is a second answer to "may this contact receive a record", and the
// two drift until a manager is refused a colleague the router happily assigns.
//
// The set, not any one column, is the subject. Plenty of SQL in the tree reads
// is_agent or seat_type for a different question — a brief that skips agents, a
// roster that hides them, a seat check on login — and matching a single column
// would drag all of them in and teach the next reader that this gate cries
// wolf. A statement asking about eligibility to RECEIVE work asks about the
// live seat AND the human AND the seat type together.
var eligibilityColumns = []string{"is_agent", "seat_type", "archived_at"}

// gatekit:fixture the defining predicate; every other reader calls it.
var assigneeEligibilityWriters = map[string]string{
	"internal/platform/auth/assignscope.go": "the rule itself: an active human seat that can write",
}

func TestSeatEligibilityIsAskedTheSameWayEverywhereItIsAsked(t *testing.T) {
	t.Parallel()
	found := map[string][]string{}
	root := filepath.Join(gatekit.ModuleRoot(t), "internal")
	err := gatekit.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		body, readErr := gatekit.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		text := string(body)
		// Only SQL that reads app_user can be asking this question; a Go
		// struct field named is_agent is not a seat-eligibility decision.
		if !strings.Contains(text, "app_user") {
			return nil
		}
		rel := "internal/" + strings.TrimPrefix(filepath.ToSlash(strings.TrimPrefix(path, root)), "/")
		// All of them, in one file, plus the active-status clause, spelled out
		// or called through the shared live-member helper. That conjunction is
		// the question, and any subset is a different one.
		if !strings.Contains(text, "status = 'active'") && !strings.Contains(text, "workforce.LiveMemberSQL(") {
			return nil
		}
		hits := []string{}
		for _, col := range eligibilityColumns {
			if strings.Contains(text, col) {
				hits = append(hits, col)
			}
		}
		if len(hits) == len(eligibilityColumns) {
			found[rel] = hits
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking the tree: %v", err)
	}

	for file := range found {
		if _, allowed := assigneeEligibilityWriters[file]; !allowed {
			t.Errorf("%s asks whether a seat may receive work (%v), but only these may: %v.\n"+
				"Call auth.EnsureAssignee rather than asking again — a third reading of this rule is a "+
				"third answer, and the one that disagrees is found by a user, not by a test.",
				file, found[file], keysOf(assigneeEligibilityWriters))
		}
	}
	for file, why := range assigneeEligibilityWriters {
		if len(found[file]) == 0 {
			t.Errorf("%s is registered as asking about seat eligibility (%s) and no longer does: "+
				"if the question moved, move this row; if it stopped being asked, the OTHER writer is "+
				"now alone and this gate is holding nothing.", file, why)
		}
	}
	// Under-recognition is the failure this cannot have: a census that reads a
	// smaller tree reports PASS with nothing to notice.
	if len(found) < len(assigneeEligibilityWriters) {
		t.Fatalf("the scan found %d file(s) asking about seat eligibility and the register names %d: "+
			"the scan has stopped seeing its own subjects", len(found), len(assigneeEligibilityWriters))
	}
}

func keysOf(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// Each predicate keeps every clause of its rule. The seat rule refuses a seat
// that cannot do the work, and the run-as rule adds the caller's own write
// scope. Dropping a clause widens who may be handed a record or acted for,
// with no test naming what was widened.
func TestTheAssigneePredicatesKeepEveryClauseOfTheirRule(t *testing.T) {
	t.Parallel()
	path := filepath.Join(gatekit.ModuleRoot(t), "internal", "platform", "auth", "assignscope.go")
	file, err := gatekit.ParseFile(path, parser.ParseComments)
	if err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}
	src, err := gatekit.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	bodies := map[string]string{}
	ast.Inspect(file, func(n ast.Node) bool {
		if fn, ok := n.(*ast.FuncDecl); ok {
			span := gatekit.SourceFileSet().File(fn.Pos())
			bodies[fn.Name.Name] = string(src)[span.Offset(fn.Pos()):span.Offset(fn.End())]
		}
		return true
	})
	rules := map[string][]string{
		"AssigneeEligibleSQL": {"workforce.LiveMemberSQL(alias)", "NOT %[2]s.is_agent", "seat_type <> 'read'"},
		"runAsOwnerSQL":       {"AssigneeEligibleSQL(alias)", "ownerPredicate"},
	}
	for name, clauses := range rules {
		body, ok := bodies[name]
		if !ok {
			t.Errorf("%s is gone from assignscope.go: the rule it holds has moved and this gate has not", name)
			continue
		}
		for _, clause := range clauses {
			if !strings.Contains(body, clause) {
				t.Errorf("%s no longer asks %q, which widens who may be handed a record or acted for.", name, clause)
			}
		}
	}
}
