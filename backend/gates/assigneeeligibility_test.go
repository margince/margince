// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A seat's fitness to receive work has ONE spelling, wherever it is asked.
//
// eligibilityColumns are the user columns that decide it. A second reader of
// this SET is a second answer to "may this person receive a record", and the
// two drift until a manager is refused a colleague the router happily assigns.
//
// The set, not any one column, is the subject. Plenty of SQL in the tree reads
// is_agent or seat_type for a different question — a brief that skips agents, a
// roster that hides them, a seat check on login — and matching a single column
// would drag all of them in and teach the next reader that this gate cries
// wolf. A statement asking about eligibility to RECEIVE work asks about the
// live seat AND the human AND the seat type together.
var eligibilityColumns = []string{"is_agent", "seat_type", "archived_at"}

// gatekit:fixture the two places that ask whether a seat may receive work,
// each with what it is — a classification of the tree, not a cost this gate is
// paying.
//
// assigneeEligibilityWriters are the two places allowed to ask whether a seat
// may RECEIVE work, each with why it is not the other.
//
// auth.assigneeEligible is the rule itself, asked of a destination a caller
// named. people/leadrouting.go asks the same question of a configured pool
// under the system principal, where the caller-scope half of the rule has no
// meaning — routing has no caller to be scoped to. They are kept in step by
// this gate rather than by a shared helper because the platform package cannot
// reach into a module and the module must not import the predicate's private
// half.
var assigneeEligibilityWriters = map[string]string{
	"internal/platform/auth/assignscope.go": "the rule itself: an active human seat inside the caller's own write scope",
	"internal/modules/people/leadrouting.go": "the routing pool's own eligibility, system-principal so the scope half " +
		"does not apply; kept in step with the rule by this gate",
}

// A seat's fitness to receive work is spelled in exactly two places, and both
// ask the same thing.
//
// The claim in auth.assigneeEligible's comment is what this holds: the rule has
// one spelling. Before it, routing checked status and archival while
// the manual path checked status, archival, agent and seat type, so the
// machine could place a lead on a seat a person was forbidden to assign to.
// That divergence passed every gate in the tree.
func TestSeatEligibilityIsAskedTheSameWayEverywhereItIsAsked(t *testing.T) {
	t.Parallel()
	found := map[string][]string{}
	root := filepath.Join(moduleRoot(t), "internal")
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		body, readErr := os.ReadFile(path)
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
		// All of them, in one file, plus the active-status clause: that
		// conjunction is the question, and any subset is a different one.
		if !strings.Contains(text, "status = 'active'") {
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

// The rule's two halves must both appear in the predicate: a version that
// dropped the seat check while keeping the scope check would still refuse a
// stranger and happily file work on a suspended colleague.
func TestTheAssigneePredicateKeepsBothHalvesOfTheRule(t *testing.T) {
	t.Parallel()
	path := filepath.Join(moduleRoot(t), "internal", "platform", "auth", "assignscope.go")
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ParseComments)
	if err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}
	var body string
	ast.Inspect(file, func(n ast.Node) bool {
		fn, ok := n.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "assigneeEligible" {
			return true
		}
		src, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatalf("reading %s: %v", path, readErr)
		}
		body = string(src)[fn.Pos()-1 : fn.End()-1]
		return false
	})
	if body == "" {
		t.Fatal("assigneeEligible is gone from assignscope.go: the rule it holds has moved and this gate has not")
	}
	for _, half := range []string{
		"status = 'active'", "archived_at IS NULL", "NOT %[1]s.is_agent",
		"seat_type <> 'read'", "ownerPredicate",
	} {
		if !strings.Contains(body, half) {
			t.Errorf("assigneeEligible no longer asks %q. Each clause refuses a seat that cannot do the "+
				"work, or scopes the destination to the assigner's own reach; dropping one widens who may "+
				"be handed a record without any test naming what was widened.", half)
		}
	}
}
