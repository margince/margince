// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gates

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// workflowRunWrite matches a SQL string literal that writes workflow_run.
var workflowRunWrite = regexp.MustCompile(`(?i)\b(INSERT\s+INTO|UPDATE|DELETE\s+FROM)\s+workflow_run\b`)

// workflowRunWriters lists every non-test file that writes workflow_run, with
// how many SQL literals it holds. The claim and outcome writes live in
// engine_run.go; the plan-shape test (automation/runplanshape_integration_test.go)
// explains each of them. Every other writer is named here with the reason it is
// not on the (handler, idempotency_key) claim key, so a new raw write turns this
// red until it is either routed through claimRunSQL / runOutcomeSQL or named.
var workflowRunWriters = map[string]struct {
	literals int
	reason   string
}{
	"internal/modules/automation/engine_run.go":           {6, "the claim insert (claimRunSQL) and the five outcome updates (runOutcomeSQL), plan-checked by the automation integration test"},
	"internal/modules/automation/engine_blocked.go":       {3, "park, release and fail the run behind one approval; keyed on detail->>'approval_id', which has no index, and only a still-requires_approval row matches"},
	"internal/modules/privacy/erasure_approvals.go":       {2, "block the runs behind withdrawn approvals (detail->>'approval_id') and scrub the subject's text from planned/applied; both are erasure sweeps, not claim-keyed"},
	"internal/modules/automation/runprune.go":             {1, "retention delete by age and status; a bounded batch sweep, not claim-keyed"},
	"internal/compose/integration/apptest/workflowrun.go": {1, "test seeding helper that inserts a run row with chosen columns"},
}

func TestEveryWorkflowRunWriteIsTheClaimPathOrNamed(t *testing.T) {
	found := map[string]int{}
	n := walkHandWrittenGoFiles(t, "internal", func(path, text string) {
		if strings.HasSuffix(path, "_test.go") || !strings.Contains(text, "workflow_run") {
			return
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, text, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", path, err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			sql, err := strconv.Unquote(lit.Value)
			if err != nil {
				t.Fatalf("unquoting a literal in %s: %v", path, err)
			}
			if workflowRunWrite.MatchString(sql) {
				found[path]++
			}
			return true
		})
	})
	if n == 0 {
		t.Fatal("the sweep found no Go files; the root moved")
	}
	var problems []string
	for path, count := range found {
		want, named := workflowRunWriters[path]
		switch {
		case !named:
			problems = append(problems, fmt.Sprintf("%s writes workflow_run in %d SQL literal(s) and is not named: route it through claimRunSQL / runOutcomeSQL, or add it to workflowRunWriters with the reason it is not claim-keyed", path, count))
		case want.literals != count:
			problems = append(problems, fmt.Sprintf("%s holds %d workflow_run write literal(s), the list says %d (%s): a new write needs the claim path or a named reason", path, count, want.literals, want.reason))
		}
	}
	for path := range workflowRunWriters {
		if found[path] == 0 {
			problems = append(problems, path+" is named in workflowRunWriters but no longer writes workflow_run; remove it")
		}
	}
	sort.Strings(problems)
	if len(problems) > 0 {
		t.Fatal(strings.Join(problems, "\n"))
	}
}
