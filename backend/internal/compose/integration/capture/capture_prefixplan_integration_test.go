// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5"
)

type historyPrefixNode struct {
	Name      string `json:"Subplan Name"`                  //nolint:tagliatelle // Postgres plan keys.
	Relation  string `json:"Relation Name"`                 //nolint:tagliatelle // Postgres plan keys.
	Rows      int    `json:"Actual Rows"`                   //nolint:tagliatelle // Postgres plan keys.
	Loops     int    `json:"Actual Loops"`                  //nolint:tagliatelle // Postgres plan keys.
	Filtered  int    `json:"Rows Removed by Filter"`        //nolint:tagliatelle // Postgres plan keys.
	Rechecked int    `json:"Rows Removed by Index Recheck"` //nolint:tagliatelle // Postgres plan keys.
	Plans     []historyPrefixNode
}

func checkHistoryNamePlans(cost historyQueryCost, plans ...[]byte) error {
	if len(cost.args) != 1 {
		return errors.New("name plan did not retain its named arguments")
	}
	args, ok := cost.args[0].(pgx.NamedArgs)
	if !ok {
		return errors.New("name plan arguments are not named")
	}
	limit, ok := args["limit"].(int)
	if !ok {
		return errors.New("name plan did not retain its candidate limit")
	}
	seen := false
	for _, raw := range plans {
		if raw != nil {
			seen = true
			if err := checkHistoryPrefixPlan(raw, limit); err != nil {
				return err
			}
		}
	}
	if !seen {
		return errors.New("name plan has no saved execution")
	}
	return nil
}

func checkHistoryPrefixPlan(raw []byte, limit int) error {
	var plans []struct{ Plan historyPrefixNode }
	if err := json.Unmarshal(raw, &plans); err != nil {
		return err
	}
	if len(plans) != 1 || limit < 1 {
		return errors.New("expected one name plan and a positive candidate limit")
	}
	found, err := plans[0].Plan.checkPerfectPrefix(limit)
	if err != nil {
		return err
	}
	if !found {
		return errors.New("name plan did not report its perfect prefix")
	}
	return nil
}

func (n historyPrefixNode) checkPerfectPrefix(limit int) (bool, error) {
	if n.Name == "CTE perfect" {
		if n.Rows < limit {
			return true, nil
		}
		return true, n.checkContactReads(limit + 1)
	}
	for _, child := range n.Plans {
		if found, err := child.checkPerfectPrefix(limit); found || err != nil {
			return found, err
		}
	}
	return false, nil
}

func (n historyPrefixNode) checkContactReads(limit int) error {
	reads := (n.Rows + n.Filtered + n.Rechecked) * n.Loops
	if n.Relation == "contact" && reads > limit {
		return fmt.Errorf("perfect prefix visited %d contact rows for %d candidates", reads, limit)
	}
	for _, child := range n.Plans {
		if err := child.checkContactReads(limit); err != nil {
			return err
		}
	}
	return nil
}

func TestImportNamePlansCountVisitedRowsAndRequireTheirPrefix(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		plan string
		good bool
	}{
		{"bounded", `[{"Plan":{"Subplan Name":"CTE perfect","Actual Rows":2,"Plans":[{"Relation Name":"contact","Actual Rows":2,"Actual Loops":1}]}}]`, true},
		{"excluded row", `[{"Plan":{"Subplan Name":"CTE perfect","Actual Rows":2,"Plans":[{"Relation Name":"contact","Actual Rows":2,"Actual Loops":1,"Rows Removed by Filter":1}]}}]`, true},
		{"bitmap intersection", `[{"Plan":{"Subplan Name":"CTE perfect","Actual Rows":2,"Plans":[{"Relation Name":"contact","Actual Rows":20,"Actual Loops":1}]}}]`, false},
		{"discarded rows", `[{"Plan":{"Subplan Name":"CTE perfect","Actual Rows":2,"Plans":[{"Relation Name":"contact","Actual Rows":2,"Actual Loops":1,"Rows Removed by Filter":20}]}}]`, false},
		{"index rechecks", `[{"Plan":{"Subplan Name":"CTE perfect","Actual Rows":2,"Plans":[{"Relation Name":"contact","Actual Rows":2,"Actual Loops":1,"Rows Removed by Index Recheck":20}]}}]`, false},
		{"short prefix needs fallback", `[{"Plan":{"Subplan Name":"CTE perfect","Actual Rows":1,"Plans":[{"Relation Name":"contact","Actual Rows":20,"Actual Loops":1}]}}]`, true},
		{"missing prefix", `[{"Plan":{"Actual Rows":2}}]`, false},
		{"unreadable plan", `{`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := checkHistoryPrefixPlan([]byte(tc.plan), 2)
			if (err == nil) != tc.good {
				t.Fatalf("plan accepted=%v, want %v: %v", err == nil, tc.good, err)
			}
		})
	}
}

func TestImportNamePlansRequireTheMeasuredArgumentsAndAnExecution(t *testing.T) {
	t.Parallel()
	const plan = `[{"Plan":{"Subplan Name":"CTE perfect","Actual Rows":2,"Plans":[{"Relation Name":"contact","Actual Rows":2,"Actual Loops":1}]}}]`
	for _, args := range [][]any{nil, {"unnamed"}, {pgx.NamedArgs{"limit": "two"}}, {pgx.NamedArgs{"limit": 0}}} {
		if err := checkHistoryNamePlans(historyQueryCost{args: args}, []byte(plan)); err == nil {
			t.Fatalf("accepted missing or invalid measured arguments: %v", args)
		}
	}
	cost := historyQueryCost{args: []any{pgx.NamedArgs{"limit": 2}}}
	if err := checkHistoryNamePlans(cost, nil); err == nil {
		t.Fatal("accepted a missing execution plan")
	}
	if err := checkHistoryNamePlans(cost, []byte(plan)); err != nil {
		t.Fatal(err)
	}
}
