// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package automation

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/workflow"
)

// Plan nodes are read from EXPLAIN (FORMAT JSON), asserting the access method,
// the index and the columns it is probed on, never the whole plan text.
type planNode struct {
	NodeType       string     `json:"Node Type"`
	IndexName      string     `json:"Index Name"`
	IndexCond      string     `json:"Index Cond"`
	ArbiterIndexes []string   `json:"Conflict Arbiter Indexes"`
	Plans          []planNode `json:"Plans"`
}

func (n planNode) walk(visit func(planNode)) {
	visit(n)
	for _, c := range n.Plans {
		c.walk(visit)
	}
}

func (fx *autoFixture) explain(t *testing.T, sql string, args ...any) planNode {
	t.Helper()
	var raw []byte
	if err := fx.owner.QueryRow(context.Background(), "EXPLAIN (FORMAT JSON) "+sql, args...).Scan(&raw); err != nil {
		t.Fatalf("EXPLAIN failed: %v\nstatement: %s", err, sql)
	}
	var wrapped []struct {
		Plan planNode `json:"Plan"`
	}
	if err := json.Unmarshal(raw, &wrapped); err != nil || len(wrapped) != 1 {
		t.Fatalf("unreadable EXPLAIN output %s: %v", raw, err)
	}
	return wrapped[0].Plan
}

// TestRunClaimAndOutcomeStatementsReachTheirRowByTheUniqueIndex holds the cost
// of a firing without reading a clock. A firing touches workflow_run once to
// claim its row and once to record its outcome. If an outcome statement loses
// idempotency_key from its index probe, it still uses an index and still passes
// every functional test, but reads every run its handler ever made. So the plan
// must probe workflow_run_unique on BOTH columns. The statements come from the
// engine (claimRunSQL, runOutcomeSQL); the rows are written by the engine's
// own claim, then ANALYZEd, so the planner has real statistics to choose from.
func TestRunClaimAndOutcomeStatementsReachTheirRowByTheUniqueIndex(t *testing.T) {
	fx := setupAutomationDB(t)
	engine := NewWorkflowEngine(database.BindTo(fx.pool, ids.From[ids.WorkspaceKind](fx.ws)), nil)
	ctx := context.Background()

	const perHandler = 1500
	var lastKey string
	for _, name := range []string{"wf_a", "wf_b", "wf_c"} {
		h := scriptedWorkflow{name: name}
		for range perHandler {
			ev := workflow.Event{ID: ids.NewV7(), AutomationID: ids.NewV7()}
			if _, err := engine.claimRun(ctx, h, ev, emptyPlan, runStatusApplied, nil); err != nil {
				t.Fatal(err)
			}
			lastKey = runKey(h, ev)
		}
	}
	fx.exec(t, `ANALYZE workflow_run`)

	type statement struct {
		name string
		sql  string
		args []any
	}
	claimArgs := []any{"wf_c", lastKey, ids.NewV7(), `[]`, runStatusApplied, `{}`}
	statements := []statement{
		{"run claim", claimRunSQL(false), claimArgs},
		{"run claim with reclaim", claimRunSQL(true), append(append([]any{}, claimArgs...), applyRetryWindow)},
	}
	outcomes := make([]string, 0, len(runOutcomeSQL))
	for name := range runOutcomeSQL {
		outcomes = append(outcomes, string(name))
	}
	sort.Strings(outcomes)
	for _, name := range outcomes {
		sql := runOutcomeSQL[runOutcome(name)]
		args := []any{"wf_c", lastKey, `[]`, `[]`}
		statements = append(statements, statement{"run outcome " + name, sql, args[:strings.Count(sql, "$")]})
	}

	for _, s := range statements {
		t.Run(s.name, func(t *testing.T) {
			plan := fx.explain(t, s.sql, s.args...)
			var problems []string
			if strings.HasPrefix(s.name, "run claim") {
				if !slices.Contains(plan.ArbiterIndexes, "workflow_run_unique") {
					problems = append(problems, fmt.Sprintf("conflict arbiter is %v", plan.ArbiterIndexes))
				}
			} else {
				problems = append(problems, scanProblems(plan)...)
			}
			if len(problems) > 0 {
				t.Errorf("%q lost its index path: %s\nstatement: %s", s.name, strings.Join(problems, "; "), s.sql)
			}
		})
	}
}

// scanProblems wants exactly one scan of workflow_run, an Index Scan or Index
// Only Scan of workflow_run_unique whose condition names both columns.
func scanProblems(plan planNode) []string {
	var scans []planNode
	plan.walk(func(n planNode) {
		if strings.Contains(n.NodeType, "Scan") {
			scans = append(scans, n)
		}
	})
	if len(scans) != 1 {
		return []string{fmt.Sprintf("expected one scan of workflow_run, plan has %d", len(scans))}
	}
	scan := scans[0]
	var problems []string
	if scan.NodeType != "Index Scan" && scan.NodeType != "Index Only Scan" {
		problems = append(problems, "access method is "+scan.NodeType)
	}
	if scan.IndexName != "workflow_run_unique" {
		problems = append(problems, fmt.Sprintf("index is %q", scan.IndexName))
	}
	for _, col := range []string{"handler", "idempotency_key"} {
		if !strings.Contains(scan.IndexCond, col) {
			problems = append(problems, fmt.Sprintf("index condition %q does not probe %s", scan.IndexCond, col))
		}
	}
	return problems
}
