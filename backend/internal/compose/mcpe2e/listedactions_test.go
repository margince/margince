// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mcpe2e_test

import (
	"encoding/json"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/mcpe2e"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// routerAction is one action of one listed router.
type routerAction struct{ Router, Action string }

// usedRouterActions names the router actions the lane uses, each with where.
// A scenario uses an action it requires, by its verb or as
// router.action=<name>. A recorded MCP run uses the verbs it called.
func usedRouterActions(t *testing.T, specs []mcp.ToolSpec) map[routerAction][]string {
	t.Helper()
	byVerb, fields := map[string]routerAction{}, map[string]string{}
	for _, spec := range specs {
		if spec.Unlisted {
			continue
		}
		fields[spec.Name] = spec.ActionField
		for _, a := range spec.Actions {
			byVerb[a.Verb] = routerAction{spec.Name, a.Action}
		}
	}
	calls, err := mcpe2e.RequiredCalls(filepath.Join("..", "..", "..", "..", mcpe2e.ScenarioDir))
	if err != nil {
		t.Fatalf("reading the scenarios: %v", err)
	}
	used := map[routerAction][]string{}
	note := func(action routerAction, where string) {
		if !slices.Contains(used[action], where) {
			used[action] = append(used[action], where)
		}
	}
	for file, names := range calls {
		for _, name := range names {
			tool, pinned, _ := strings.Cut(name, ".")
			action, fronted := byVerb[tool]
			argument, value, pins := strings.Cut(pinned, "=")
			if !fronted && pins && fields[tool] != "" && fields[tool] == argument {
				action, fronted = routerAction{tool, value}, true
			}
			if fronted {
				note(action, file)
			}
		}
	}
	verdicts, err := mcpe2e.ReadVerdicts(filepath.Join("..", "..", "..", ".."))
	if err != nil {
		t.Fatalf("reading the verdicts: %v", err)
	}
	for _, v := range verdicts {
		if v.Door != mcpe2e.DoorMCP {
			continue
		}
		called := slices.Clone(v.ToolsCalled)
		for _, run := range v.ToolsByRun {
			called = append(called, run.Tools...)
		}
		for _, verb := range called {
			if action, fronted := byVerb[verb]; fronted {
				note(action, "a recorded run of "+v.Scenario)
			}
		}
	}
	return used
}

// Every router action the lane uses keeps its arguments in tools/list: one a
// scenario requires, or one a recorded run called. Any other action keeps only
// its enum line, and a model reads its arguments from the action catalog. The
// set names nothing the lane has stopped using, so it cannot go stale.
func TestTheToolListNamesTheArgumentsOfEveryActionTheLaneUses(t *testing.T) {
	specs := compose.NewRegistry(nil, compose.SendPath{}).Specs()
	used := usedRouterActions(t, specs)
	if len(used) == 0 {
		t.Fatal("no scenario or recorded run uses a router action, so this gate would hold nothing")
	}
	served := map[string]mcp.ToolSpec{}
	for _, spec := range specs {
		served[spec.Name] = spec
	}
	for action, scenarios := range used {
		sort.Strings(scenarios)
		router := served[action.Router]
		i := slices.IndexFunc(router.Actions, func(a mcp.ToolAction) bool { return a.Action == action.Action })
		if i < 0 {
			t.Errorf("%s names %s action %s, which the router does not have", scenarios[0], action.Router, action.Action)
			continue
		}
		listed := propertyNames(t, router.InputSchema)
		for _, argument := range propertyNames(t, served[router.Actions[i].Verb].InputSchema) {
			if !slices.Contains(listed, argument) {
				t.Errorf("%s uses %s action %s, whose argument %s tools/list leaves out. "+
					"Add the action to listedActions in backend/internal/modules/agents/routerlisting.go.",
					strings.Join(scenarios, ", "), action.Router, action.Action, argument)
			}
		}
	}
	for router, actions := range agents.ListedActions() {
		for _, action := range actions {
			if _, needed := used[routerAction{router, action}]; !needed {
				t.Errorf("listedActions keeps the arguments of %s action %s, which no scenario requires and no "+
					"recorded MCP run called. Remove it from backend/internal/modules/agents/routerlisting.go.", router, action)
			}
		}
	}
}

func propertyNames(t *testing.T, schema json.RawMessage) []string {
	t.Helper()
	var doc struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal(schema, &doc); err != nil {
		t.Fatalf("reading an input schema: %v", err)
	}
	names := make([]string, 0, len(doc.Properties))
	for name := range doc.Properties {
		names = append(names, name)
	}
	return names
}
