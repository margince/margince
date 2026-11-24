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
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
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
		`"shared":{"type":"string","enum":["a"]},"listed_only":{"type":"integer","description":"Count it with rare_verb."}},"additionalProperties":false}`)
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

// properties are the members of the router's schema, as the MCP listing shows
// it, or as every other reader gets it.
func (f *rareListingFixture) properties(t *testing.T, listed bool) map[string]json.RawMessage {
	t.Helper()
	spec, _ := f.r.Spec(f.router)
	if listed {
		spec = f.r.ForListing(spec)
	}
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

// An argument only rare actions take is left out of the MCP listing. One a
// listed action takes stays with the shape every action that takes it gives it.
// The registered schema, which the in-app runner reads, keeps them all.
func TestARouterListsOnlyTheArgumentsItsListedActionsTake(t *testing.T) {
	f := newRareListingFixture(t)
	properties, registered := f.properties(t, true), f.properties(t, false)
	for _, gone := range []string{"rare_only", "route_only", "if_version", "id"} {
		if _, listed := properties[gone]; listed {
			t.Errorf("%s, which only rare actions take, is listed: %s", gone, properties[gone])
		}
		if _, kept := registered[gone]; !kept {
			t.Errorf("%s is left out of the registered schema, so the in-app runner cannot send it", gone)
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
	for _, says := range []string{
		`"action":"` + rareInProcess + `"`, `"rare_only"`, `"action":"` + f.listed + `"`, `"listed_only"`,
		`"action":"` + rareRouted + `"`, `"name":"route_only"`, `"enum":["x","y"]`,
	} {
		if !strings.Contains(string(whole), says) {
			t.Errorf("the catalog %s does not name %s", whole, says)
		}
	}
	describe, _ := f.r.tools[describeRouterActionsName].(describeRouterActions)
	one, err := describe.catalog(f.router, rareInProcess, nil)
	if err != nil || !strings.Contains(string(one), `"rare_only"`) || strings.Contains(string(one), "listed_only") {
		t.Errorf("the catalog for %s alone = %s, %v", rareInProcess, one, err)
	}
}

// A caller reads the catalog of what its own tools/list offers. A read-only
// passport is offered no write router, so asking for one is refused as a
// router the catalog does not have.
func TestTheActionCatalogAnswersOnlyForWhatTheCallerIsOffered(t *testing.T) {
	f := newRareListingFixture(t)
	describe, _ := f.r.tools[describeRouterActionsName].(describeRouterActions)
	readOnly := principal.WithActor(principal.WithWorkspaceID(context.Background(), ids.NewV7()), principal.Principal{
		Type: principal.PrincipalAgent, ID: "agent:r", OnBehalfOf: ids.NewV7(), SeatType: principal.SeatFull,
		Scopes: principal.NewScopeSet(principal.ScopeRead), Permissions: everyRecordedGrant(),
	})
	if slices.ContainsFunc(f.r.Offered(readOnly), func(spec mcp.ToolSpec) bool { return spec.Name == f.router }) {
		t.Fatalf("a read-only passport is offered the write router %s, so this case tests nothing", f.router)
	}
	var bad *BadArgsError
	if _, err := describe.Handle(readOnly, json.RawMessage(`{"router":"`+f.router+`"}`)); !errors.As(err, &bad) || bad.Field != catalogRouterArg {
		t.Errorf("a read-only passport asking for %s's actions answered %v, want the refusal for a router it is not offered", f.router, err)
	}
	out, err := describe.Handle(agentCtx(), json.RawMessage(`{"router":"`+f.router+`"}`))
	if err != nil || !strings.Contains(string(out), rareInProcess) {
		t.Errorf("a write passport asking for %s's actions = %s, %v", f.router, out, err)
	}
}

// A refused call to a rare action says where its arguments go and where to
// read them. The listing a client holds does not name them.
func TestARareActionsRefusalSaysWhereItsArgumentsGo(t *testing.T) {
	f := newRareListingFixture(t)
	catalog := "read_workspace with topic=actions, router=" + f.router + " and action="
	for _, c := range []struct{ call, want string }{
		{`{"action":"` + rareRouted + `","request_body":{"id":"d1"}}`, "move `id` out of request_body: this action takes it at the top level, beside `action`"},
		{`{"action":"` + rareInProcess + `"}`, "add `rare_only` at the top level, beside `action`"},
		{`{"action":"` + rareRouted + `","id":"d1","entity_type":"deal"}`, "leave out `entity_type`, which " + rareRouted + " does not take"},
	} {
		_, err := f.r.InvokeServed(agentCtx(), f.router, json.RawMessage(c.call))
		var bad *BadArgsError
		if !errors.As(err, &bad) {
			t.Errorf("%s answered %v, want a refusal", c.call, err)
			continue
		}
		action := rareRouted
		if strings.Contains(c.call, rareInProcess) {
			action = rareInProcess
		}
		if !strings.Contains(bad.Guidance, c.want) || !strings.Contains(bad.Guidance, catalog+action) {
			t.Errorf("%s was refused with %q, want %q and the catalog to read", c.call, bad.Guidance, c.want)
		}
		if strings.Contains(err.Error(), "tools/list") {
			t.Errorf("%s was refused pointing at tools/list, which leaves this action's arguments out: %v", c.call, err)
		}
	}
}

// An action whose call its user confirms in the conversation takes confirm_code,
// so the router lists it for that action. The surface splices it into the
// target, so the router must read it from there.
func TestARouterListsConfirmCodeForTheActionsThatTakeIt(t *testing.T) {
	r := NewRegistry(nil, auth.NewGate(fullSeatAuthority{}))
	waits := &argsTool{spec: targetSpec("waits_for_user", principal.ScopeWrite, mcp.TierConfirmationRequired)}
	waits.spec.WaitsEvenForItsUser = true
	r.Register(waits)
	r.Register(&argsTool{spec: targetSpec("runs_now", principal.ScopeWrite, mcp.TierAutoExecute)})
	r.Register(fakeRouter{name: "fake_confirm", targets: map[string]string{"confirm": "waits_for_user", "run": "runs_now"}})
	spec, _ := r.Spec("fake_confirm")
	var doc struct {
		Properties map[string]struct {
			Description string `json:"description"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(spec.InputSchema, &doc); err != nil {
		t.Fatal(err)
	}
	code, listed := doc.Properties[confirmCodeArg]
	if !listed || !strings.HasPrefix(code.Description, "confirm only") {
		t.Errorf("the router lists confirm_code as %+v (listed %v), want it scoped to the confirm action", code, listed)
	}
}

// The MCP listing names a target verb as its router's action, as every other
// listing does. It never names a tool the listing does not offer.
func TestTheShortenedListingNamesATargetVerbAsItsRoutersAction(t *testing.T) {
	f := newRareListingFixture(t)
	spec, _ := f.r.Spec(f.router)
	catalog, err := f.r.ActionCatalog(f.router)
	if err != nil {
		t.Fatal(err)
	}
	for name, text := range map[string]string{
		"listing": string(f.r.ForListing(f.r.Listed(spec)).InputSchema), "action catalog": string(catalog),
	} {
		if strings.Contains(text, "with rare_verb") || !strings.Contains(text, rareInProcess) {
			t.Errorf("the %s words the target verb as a tool of its own: %s", name, text)
		}
	}
}

// A router that picks its action by another member says so where it says how
// to call it.
func TestTheCallGuideNamesTheMemberARouterPicksItsActionBy(t *testing.T) {
	for _, tc := range []struct {
		selector func(string) string
		want     string
	}{
		{func(string) string { return "topic" }, "with topic and"},
		{nil, "with action and"},
	} {
		d := describeRouterActions{selector: tc.selector}
		if got := howToCall(d.selectorOf("read_workspace")); !strings.Contains(got, tc.want) {
			t.Errorf("the call guide reads %q, want it to say %q", got, tc.want)
		}
	}
}
