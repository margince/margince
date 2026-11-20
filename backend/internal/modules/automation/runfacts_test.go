// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package automation

import (
	"encoding/json"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	"github.com/margince/margince/backend/internal/shared/ports/workflow"
)

func TestARunWhoseTaskAnotherRuleMadeNamesNoCreatedTask(t *testing.T) {
	deal := datasource.EntityRef{Type: "deal", ID: ids.NewV7()}
	trace := []workflow.Action{{
		Kind: workflow.ActionCreateTask, Target: deal, Args: json.RawMessage(`{"subject":"Send the proposal"}`), Deduplicated: true,
	}}
	var run crmcontracts.AutomationRun
	describeRun(AutomationRunRecord{Status: runStatusApplied, Planned: trace, Applied: trace, TriggerEvent: ids.NewV7()}, &run)

	if run.ActionKinds == nil || len(*run.ActionKinds) != 0 {
		t.Errorf("action_kinds = %v, want empty: another rule made the task", run.ActionKinds)
	}
	if run.Created != nil {
		t.Errorf("created = %+v, want absent: this run created nothing", run.Created)
	}
	if run.Target == nil || ids.UUID(run.Target.Id) != deal.ID {
		t.Errorf("target = %+v, want the deal the run acted on", run.Target)
	}
}

func TestARunTraceDecodesOnlyAnActionList(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		actions int
		fails   bool
	}{
		{name: "a run that never recorded it", raw: ""},
		{name: "a JSON null", raw: "null"},
		{name: "an empty list", raw: "[]"},
		{name: "one action", raw: `[{"kind":"create_task"}]`, actions: 1},
		{name: "an object", raw: "{}", fails: true},
		{name: "a truncated list", raw: `[{"kind":`, fails: true},
	}
	for _, tc := range cases {
		actions, err := decodeRunTrace("applied", []byte(tc.raw))
		if (err != nil) != tc.fails {
			t.Errorf("%s: err = %v, want failure %v", tc.name, err, tc.fails)
		}
		if len(actions) != tc.actions {
			t.Errorf("%s: read %d actions, want %d", tc.name, len(actions), tc.actions)
		}
	}
}
