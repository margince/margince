// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// rareListingFixture is a router the action catalog describes. It has one
// listed action and two rare ones: a route verb and one it runs in process.
type rareListingFixture struct {
	r               *Registry
	router, listed  string
	common, ranRare *argsTool
	routed          *RouteCall
}

const (
	rareRouted    = "rare_routed"
	rareInProcess = "rare_in_process"
)

func newRareListingFixture(t *testing.T) *rareListingFixture {
	t.Helper()
	sets := ListedActions()
	router := slices.Sorted(maps.Keys(sets))[0]
	f := &rareListingFixture{r: NewRegistry(nil, auth.NewGate(fullSeatAuthority{})), router: router, listed: sets[router][0]}
	f.common = &argsTool{spec: targetSpec("common_verb", principal.ScopeWrite, mcp.TierAutoExecute)}
	f.common.spec.InputSchema = json.RawMessage(`{"type":"object","properties":{` +
		`"shared":{"type":"string","enum":["a"]},"listed_only":{"type":"integer"}},"additionalProperties":false}`)
	f.ranRare = &argsTool{spec: targetSpec("rare_verb", principal.ScopeWrite, mcp.TierAutoExecute)}
	f.ranRare.spec.InputSchema = json.RawMessage(`{"type":"object","required":["rare_only"],"properties":{` +
		`"shared":{"type":"string","enum":["b"]},"rare_only":{"type":"string"}},"additionalProperties":false}`)
	f.r.Register(f.common)
	f.r.Register(f.ranRare)
	verb := revertStageVerb
	verb.Router, verb.Action, verb.AskUserFirst = router, rareRouted, false
	verb.Query = []ViewQuery{{Name: "route_only", Type: "string", Enum: []string{"x", "y"}}}
	RegisterRouteVerbs(f.r, []RouteVerb{verb}, func(_ context.Context, call RouteCall) (json.RawMessage, error) {
		f.routed = &call
		return json.RawMessage(`{"status":200}`), nil
	})
	f.r.Register(fakeRouter{name: router, targets: map[string]string{f.listed: "common_verb", rareInProcess: "rare_verb"}})
	return f
}

func (f *rareListingFixture) listedSchema(t *testing.T) map[string]json.RawMessage {
	t.Helper()
	spec, _ := f.r.Spec(f.router)
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(spec.InputSchema, &doc); err != nil {
		t.Fatal(err)
	}
	if _, closed := doc["additionalProperties"]; closed {
		t.Errorf("the router's schema %s closes itself to members it does not name", spec.InputSchema)
	}
	var properties map[string]json.RawMessage
	if err := json.Unmarshal(doc["properties"], &properties); err != nil {
		t.Fatal(err)
	}
	return properties
}

// An argument only rare actions take is left out of the listing. One a listed
// action takes stays with the shape every action that takes it gives it.
func TestARouterListsOnlyTheArgumentsItsListedActionsTake(t *testing.T) {
	properties := newRareListingFixture(t).listedSchema(t)
	for _, gone := range []string{"rare_only", "route_only", "if_version", "id"} {
		if _, listed := properties[gone]; listed {
			t.Errorf("%s, which only rare actions take, is listed: %s", gone, properties[gone])
		}
	}
	if _, listed := properties["listed_only"]; !listed {
		t.Error("listed_only, which the listed action takes, is left out")
	}
	if !strings.Contains(string(properties["shared"]), `"enum":["a","b"]`) {
		t.Errorf("shared is listed as %s, want the words of every action that takes it", properties["shared"])
	}
	if !strings.Contains(string(properties[actionArg]), rareInProcess) || !strings.Contains(string(properties[actionArg]), rareRouted) {
		t.Errorf("the action enum %s lacks a rare action", properties[actionArg])
	}
}

// The listing leaves a rare action's arguments out, and the call still carries
// them to the target, whose own schema checks them.
func TestARareActionsArgumentsReachItsTarget(t *testing.T) {
	f := newRareListingFixture(t)
	if _, err := f.r.InvokeServed(agentCtx(), f.router, json.RawMessage(`{"action":"`+rareInProcess+`","rare_only":"r"}`)); err != nil {
		t.Fatalf("a rare in-process action was refused: %v", err)
	}
	if !strings.Contains(string(f.ranRare.args), `"rare_only":"r"`) {
		t.Errorf("the target was handed %s, want rare_only", f.ranRare.args)
	}
	if _, err := f.r.InvokeServed(agentCtx(), f.router, json.RawMessage(`{"action":"`+rareRouted+`","id":"d1","route_only":"y"}`)); err != nil {
		t.Fatalf("a rare routed action was refused: %v", err)
	}
	if f.routed == nil || f.routed.Path["id"] != "d1" || f.routed.Query.Get("route_only") != "y" {
		t.Errorf("the route was handed %+v, want id and route_only", f.routed)
	}
	var bad *BadArgsError
	_, err := f.r.InvokeServed(agentCtx(), f.router, json.RawMessage(`{"action":"`+rareInProcess+`"}`))
	if !errors.As(err, &bad) {
		t.Errorf("a rare action missing its required argument answered %v, want the target's own refusal", err)
	}
	_, err = f.r.InvokeServed(agentCtx(), f.router, json.RawMessage(`{"action":"`+rareRouted+`","id":"d1","listed_only":1}`))
	if !errors.As(err, &bad) {
		t.Errorf("a rare action handed an argument it does not take answered %v, want the target's own refusal", err)
	}
}

// The catalog names the arguments of every action, routed or run in process,
// with or without one action asked for.
func TestTheActionCatalogNamesEveryActionsArguments(t *testing.T) {
	f := newRareListingFixture(t)
	whole, err := f.r.ActionCatalog(f.router)
	if err != nil {
		t.Fatal(err)
	}
	for _, says := range []string{`"action":"` + rareInProcess + `"`, `"rare_only"`, `"action":"` + f.listed + `"`, `"listed_only"`,
		`"action":"` + rareRouted + `"`, `"name":"route_only"`, `"enum":["x","y"]`} {
		if !strings.Contains(string(whole), says) {
			t.Errorf("the catalog %s does not name %s", whole, says)
		}
	}
	describe, _ := f.r.tools[describeRouterActionsName].(describeRouterActions)
	one, err := describe.catalog(f.router, rareInProcess)
	if err != nil || !strings.Contains(string(one), `"rare_only"`) || strings.Contains(string(one), "listed_only") {
		t.Errorf("the catalog for %s alone = %s, %v", rareInProcess, one, err)
	}
}
