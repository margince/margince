// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// argsTool is an unlisted target that remembers what it was handed.
type argsTool struct {
	spec  mcp.ToolSpec
	calls int
	args  json.RawMessage
}

func (a *argsTool) Spec() mcp.ToolSpec { return a.spec }
func (a *argsTool) Handle(_ context.Context, in json.RawMessage) (json.RawMessage, error) {
	a.calls++
	a.args = in
	return json.RawMessage(`{"ok":true}`), nil
}

func targetSpec(name string, scope principal.Scope, tier mcp.RiskTier) mcp.ToolSpec {
	return mcp.ToolSpec{
		Name: name, Title: name, Version: testToolVersion, Description: describedForRegistration,
		RequiredScope: scope, Tier: tier, Unlisted: true,
		InputSchema: json.RawMessage(`{"type":"object","properties":{"x":{"type":"integer"}}}`),
	}
}

type fakeRouter struct {
	name    string
	targets map[string]string
	schema  string
}

func (f fakeRouter) Spec() mcp.ToolSpec {
	spec := mcp.ToolSpec{Name: f.name, Title: f.name, Version: testToolVersion, Description: describedForRegistration}
	if f.schema != "" {
		spec.InputSchema = json.RawMessage(f.schema)
	}
	return spec
}

func (fakeRouter) Handle(context.Context, json.RawMessage) (json.RawMessage, error) {
	return nil, errors.New("a router is resolved before any handler runs")
}
func (fakeRouter) routeField() string                { return "action" }
func (f fakeRouter) routeTargets() map[string]string { return f.targets }

// unlistedStaging is the confirm-first stagingTool, fronted by a router.
type unlistedStaging struct{ *stagingTool }

func (u unlistedStaging) Spec() mcp.ToolSpec {
	spec := u.stagingTool.Spec()
	spec.Unlisted = true
	return spec
}

// recordingStageApprovals remembers the kind a call staged and was redeemed for.
type recordingStageApprovals struct {
	fakeApprovals
	kind        string
	redeemedFor string
}

func (a *recordingStageApprovals) StageCall(ctx context.Context, in StageRequest) (ids.ApprovalID, bool, error) {
	a.kind = in.Tool
	return a.fakeApprovals.StageCall(ctx, in)
}

func (a *recordingStageApprovals) Redeem(ctx context.Context, id ids.ApprovalID, tool, diffHash string) (int64, bool, error) {
	a.redeemedFor = tool
	return a.fakeApprovals.Redeem(ctx, id, tool, diffHash)
}

func routedRegistry(t *testing.T) (*Registry, *argsTool, *argsTool) {
	t.Helper()
	r := NewRegistry(nil, auth.NewGate(fullSeatAuthority{}))
	a := &argsTool{spec: targetSpec("fake_a", principal.ScopeWrite, mcp.TierAutoExecute)}
	b := &argsTool{spec: targetSpec("fake_b", principal.ScopeWrite, mcp.TierAutoExecute)}
	r.Register(a)
	r.Register(b)
	r.Register(fakeRouter{name: "fake_ab", targets: map[string]string{"a": "fake_a", "b": "fake_b"}})
	return r, a, b
}

func TestRouterDispatchesToTheTarget(t *testing.T) {
	r, a, b := routedRegistry(t)
	served, err := r.InvokeServed(agentCtx(), "fake_ab", json.RawMessage(`{"action":"a","x":1}`))
	if err != nil {
		t.Fatalf("routed call refused: %v", err)
	}
	if served.Tool != "fake_a" || served.Router != "fake_ab" {
		t.Errorf("served %q through %q, want fake_a through fake_ab", served.Tool, served.Router)
	}
	if a.calls != 1 || b.calls != 0 {
		t.Fatalf("calls a=%d b=%d, want the call on fake_a alone", a.calls, b.calls)
	}
	if string(a.args) != `{"x":1}` {
		t.Errorf("target was handed %s, want the arguments without the route field", a.args)
	}
}

func TestRouterRefusesABadChoice(t *testing.T) {
	for name, in := range map[string]string{
		"missing":    `{"x":1}`,
		"unknown":    `{"action":"c"}`,
		"not a word": `{"action":1}`,
		"not json":   `{"action":`,
	} {
		t.Run(name, func(t *testing.T) {
			r, a, b := routedRegistry(t)
			_, err := r.InvokeServed(agentCtx(), "fake_ab", json.RawMessage(in))
			if _, bad := errors.AsType[*BadArgsError](err); !bad {
				t.Fatalf("answered %v, want a bad-arguments refusal", err)
			}
			if name != "not json" && !strings.Contains(err.Error(), "a, b") {
				t.Errorf("refusal %q does not name the choices", err)
			}
			if strings.Contains(err.Error(), "unmarshal") || strings.Contains(err.Error(), "json:") {
				t.Errorf("refusal %q echoes the decoder", err)
			}
			if a.calls+b.calls != 0 {
				t.Error("a refused call reached a handler")
			}
		})
	}
}

// The retry claim is the target's, over the call the target sees. A key spent
// through the router and the same call made by the old name are one call.
func TestRouterKeepsReservedArguments(t *testing.T) {
	claimed := func(name, in string) string {
		claims := &recordingClaims{verdict: Claim{State: ClaimFresh, Attempt: freshAttempt}}
		r := NewRegistry(nil, auth.NewGate(fullSeatAuthority{}), WithIdempotency(claims))
		r.Register(&argsTool{spec: targetSpec("fake_a", principal.ScopeWrite, mcp.TierAutoExecute)})
		r.Register(fakeRouter{name: "fake_ab", targets: map[string]string{"a": "fake_a"}})
		if _, err := r.InvokeServed(agentCtx(), name, json.RawMessage(in)); err != nil {
			t.Fatalf("keyed call to %s refused: %v", name, err)
		}
		if len(claims.claimed) != 1 {
			t.Fatalf("claims %v, want one", claims.claimed)
		}
		return claims.claimed[0]
	}
	routed := claimed("fake_ab", `{"action":"a","x":1,"idempotency_key":"k-1"}`)
	direct := claimed("fake_a", `{"x":1,"idempotency_key":"k-1"}`)
	if routed != direct || !strings.HasPrefix(routed, "fake_a/k-1/") {
		t.Errorf("routed claim %q, direct claim %q: want one claim under fake_a", routed, direct)
	}
}

func TestUnlistedToolIsInvocableButNotOffered(t *testing.T) {
	r, a, _ := routedRegistry(t)
	var offered []string
	for _, spec := range r.Offered(agentCtx()) {
		offered = append(offered, spec.Name)
	}
	if strings.Join(offered, ",") != "fake_ab" {
		t.Errorf("offered %v, want the router alone", offered)
	}
	if _, err := r.Invoke(agentCtx(), "fake_a", json.RawMessage(`{"x":1}`)); err != nil || a.calls != 1 {
		t.Fatalf("a client holding the old name could not call it: %v", err)
	}
}

func TestRegisterDerivesTheRouterSpec(t *testing.T) {
	r := NewRegistry(nil, nil)
	big := targetSpec("fake_a", principal.ScopeWrite, mcp.TierAutoExecute)
	big.MaxArgsBytes = 4 << 20
	big.InputSchema = json.RawMessage(`{"type":"object","required":["x"],"properties":{"x":{"type":"integer"}}}`)
	small := targetSpec("fake_b", principal.ScopeWrite, mcp.TierAutoExecute)
	small.InputSchema = json.RawMessage(`{"type":"object","required":["x"],"properties":{"x":{"type":"integer"},"y":{"type":"object"}}}`)
	small.UnkeyedArguments = map[string]string{"$.y": "the caller's own keys"}
	r.Register(&argsTool{spec: big})
	r.Register(&argsTool{spec: small})
	r.Register(fakeRouter{
		name: "fake_ab", targets: map[string]string{"b": "fake_b", "a": "fake_a"},
		schema: `{"type":"object","properties":{"action":{"description":"which one"},"x":{"description":"what x means for every action"}}}`,
	})

	spec, _ := r.Spec("fake_ab")
	if spec.RequiredScope != principal.ScopeWrite || spec.Tier != mcp.TierAutoExecute {
		t.Errorf("scope/tier = %s/%v, want the targets'", spec.RequiredScope, spec.Tier)
	}
	if spec.MaxArgsBytes != 4<<20 {
		t.Errorf("MaxArgsBytes = %d, want the largest target's", spec.MaxArgsBytes)
	}
	if spec.OutputSchema != nil {
		t.Errorf("a router advertised an output shape: %s", spec.OutputSchema)
	}
	var schema struct {
		Required   []string                   `json:"required"`
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal(spec.InputSchema, &schema); err != nil {
		t.Fatal(err)
	}
	var route struct {
		Type string   `json:"type"`
		Enum []string `json:"enum"`
	}
	if err := json.Unmarshal(schema.Properties["action"], &route); err != nil || route.Type != schemaString ||
		strings.Join(route.Enum, ",") != "a,b" {
		t.Errorf("route field = %s, want a string enum of the sorted choices", schema.Properties["action"])
	}
	if _, ok := schema.Properties["y"]; !ok {
		t.Error("property y of a target is missing")
	}
	if x := string(schema.Properties["x"]); !strings.Contains(x, "every action") || !strings.Contains(x, "integer") {
		t.Errorf("x = %s, want the target's shape with the router's wording", x)
	}
	if !strings.Contains(string(schema.Properties["action"]), "which one") {
		t.Errorf("action = %s, want the router's own description kept", schema.Properties["action"])
	}
	if strings.Join(schema.Required, ",") != "action,x" {
		t.Errorf("required = %v, want the route field and what every target requires", schema.Required)
	}
	if spec.UnkeyedArguments["$.y"] != "the caller's own keys" {
		t.Errorf("unkeyed = %v, want the target's declaration carried to the argument it advertises", spec.UnkeyedArguments)
	}
}

func TestRegisterRefusesABadRouter(t *testing.T) {
	setup := func() *Registry {
		r := NewRegistry(nil, nil)
		r.Register(&argsTool{spec: targetSpec("fake_a", principal.ScopeWrite, mcp.TierAutoExecute)})
		r.Register(&argsTool{spec: targetSpec("fake_r", principal.ScopeRead, mcp.TierAutoExecute)})
		r.Register(&argsTool{spec: targetSpec("fake_c", principal.ScopeWrite, mcp.TierConfirmationRequired)})
		listed := targetSpec("fake_l", principal.ScopeWrite, mcp.TierAutoExecute)
		listed.Unlisted = false
		r.Register(&argsTool{spec: listed})
		r.Register(fakeRouter{name: "fake_router", targets: map[string]string{"a": "fake_a"}})
		return r
	}
	for why, targets := range map[string]map[string]string{
		"a target nobody registered":       {"a": "fake_a", "z": "nope"},
		"a target that is itself a router": {"a": "fake_a", "r": "fake_router"},
		"two scopes behind one tool":       {"a": "fake_a", "r": "fake_r"},
		"a target still offered":           {"a": "fake_a", "l": "fake_l"},
	} {
		mustPanic(t, why, func() {
			setup().Register(fakeRouter{name: "bad", targets: targets})
		})
	}
}

// One router may front actions of different tiers. It lists as decided per
// call, names each action's governance, and admits each at its own verb's tier.
func TestARouterAdmitsEachActionAtItsOwnTier(t *testing.T) {
	approvals := &recordingStageApprovals{}
	r := NewRegistry(approvals, auth.NewGate(fullSeatAuthority{}))
	quick := &argsTool{spec: targetSpec("fake_a", principal.ScopeWrite, mcp.TierAutoExecute)}
	r.Register(quick)
	r.Register(unlistedStaging{&stagingTool{target: ids.NewV7()}})
	r.Register(fakeRouter{name: "mixed", targets: map[string]string{"a": "fake_a", "send": "send_it"}})

	spec, _ := r.Spec("mixed")
	if spec.Tier != mcp.TierDynamic || len(spec.Actions) != 2 {
		t.Fatalf("the router lists tier %v with actions %+v, want decided per call over two", spec.Tier, spec.Actions)
	}
	described := DescribeForClient(spec)
	if !strings.Contains(described, "runs immediately for a") || !strings.Contains(described, "a human approves first for send") {
		t.Errorf("the governance line does not name each action's tier: %s", described)
	}
	if _, err := r.InvokeServed(agentCtx(), "mixed", json.RawMessage(`{"action":"a","x":1}`)); err != nil || quick.calls != 1 {
		t.Fatalf("the 🟢 action answered %v after %d calls, want it run at once", err, quick.calls)
	}
	if _, err := r.InvokeServed(agentCtx(), "mixed", json.RawMessage(`{"action":"send","body":"hi"}`)); !errors.Is(err, apperrors.ErrRequiresApproval) || approvals.kind != "send_it" {
		t.Fatalf("the 🟡 action answered %v, staged %q; want it staged under send_it", err, approvals.kind)
	}
}

// The approval kind is the tool name, so a staging target must stage, and be
// redeemed, under its own name however it was reached.
func TestRouterStagesAndRedeemsUnderTheTargetsKind(t *testing.T) {
	approvals := &recordingStageApprovals{}
	r := NewRegistry(approvals, auth.NewGate(fullSeatAuthority{}))
	tool := &stagingTool{target: ids.NewV7()}
	r.Register(unlistedStaging{tool})
	r.Register(fakeRouter{name: "send_router", targets: map[string]string{"send": "send_it"}})

	served, err := r.InvokeServed(agentCtx(), "send_router", json.RawMessage(`{"action":"send","body":"hi"}`))
	if !errors.Is(err, apperrors.ErrRequiresApproval) {
		t.Fatalf("answered %v, want a staged refusal", err)
	}
	if served.Tool != "send_it" || approvals.kind != "send_it" {
		t.Fatalf("served %q, staged kind %q: both must be the target", served.Tool, approvals.kind)
	}
	if strings.Contains(string(approvals.change), "action") {
		t.Errorf("staged change %s carries the route field", approvals.change)
	}
	retry := `{"action":"send","body":"hi","approval_id":"` + approvals.staged.String() + `"}`
	if _, err := r.InvokeServed(agentCtx(), "send_router", json.RawMessage(retry)); err != nil {
		t.Fatalf("redeeming through the router refused: %v", err)
	}
	if tool.calls != 1 {
		t.Errorf("handler ran %d times, want once after redemption", tool.calls)
	}
	if approvals.redeemedFor != "send_it" {
		t.Errorf("redeemed for %q, want the target", approvals.redeemedFor)
	}
}

func TestRouterTaskStoresTheTarget(t *testing.T) {
	approvals := &fakeApprovals{}
	store := &fakeTasks{tasks: map[ids.UUID]Task{}, claimed: map[ids.UUID]bool{}, approvals: approvals}
	registry := NewRegistry(approvals, auth.NewGate(fullSeatAuthority{}))
	registry.Register(unlistedStaging{&stagingTool{target: ids.NewV7()}})
	registry.Register(fakeRouter{name: "send_router", targets: map[string]string{"send": "send_it"}})
	s := NewDispatcher(registry, bindAuthenticated, "margince-crm", "test").
		WithLogger(discardLog()).WithTasks(store, approvals)
	out := s.call(agentCtx(), json.RawMessage(`{"name":"send_router","arguments":{"action":"send","body":"hi"}}`), taskCapableFraming())
	if _, minted := out.(createTaskResult); !minted {
		t.Fatalf("answered %T, want a task", out)
	}
	if len(store.created) != 1 || store.created[0].Tool != "send_it" {
		t.Fatalf("created %+v, want one task for send_it", store.created)
	}
	if strings.Contains(string(store.approvals.change), "action") {
		t.Errorf("the task re-runs %s, which carries the route field", store.approvals.change)
	}
}

func TestRouterResultIsCheckedAgainstTheTarget(t *testing.T) {
	r := NewRegistry(nil, auth.NewGate(fullSeatAuthority{}))
	spec := targetSpec("fake_a", principal.ScopeWrite, mcp.TierAutoExecute)
	spec.OutputSchema = json.RawMessage(`{"type":"object","required":["ok"],"properties":{"ok":{"type":"boolean"}}}`)
	r.Register(&argsTool{spec: spec})
	r.Register(fakeRouter{name: "fake_router", targets: map[string]string{"a": "fake_a"}})
	s := NewDispatcher(r, bindAuthenticated, "margince-crm", "test").WithLogger(discardLog())
	out := s.call(agentCtx(), json.RawMessage(`{"name":"fake_router","arguments":{"action":"a"}}`), taskCapableFraming())
	res, ok := out.(map[string]any)
	if !ok || res["structuredContent"] == nil {
		t.Fatalf("answered %v, want structured content checked against fake_a's schema", out)
	}
}

func TestRouteTargetsNamesARoutersTargets(t *testing.T) {
	r, _, _ := routedRegistry(t)
	if targets, ok := r.RouteTargets("fake_ab"); !ok || strings.Join(targets, ",") != "fake_a,fake_b" {
		t.Errorf("RouteTargets(fake_ab) = %v, %v; want fake_a,fake_b", targets, ok)
	}
	if _, ok := r.RouteTargets("fake_a"); ok {
		t.Error("an ordinary tool reported as a router")
	}
}

func shapedTarget(name, properties string) *argsTool {
	spec := targetSpec(name, principal.ScopeWrite, mcp.TierAutoExecute)
	spec.InputSchema = json.RawMessage(`{"type":"object","properties":` + properties + `}`)
	return &argsTool{spec: spec}
}

// Two targets naming one argument with different words advertise every word.
// A call meant for either is then not refused by a client holding the schema.
func TestARoutedArgumentCarriesEveryTargetsWords(t *testing.T) {
	r := NewRegistry(nil, nil)
	r.Register(shapedTarget("fake_a", `{"color":{"type":"string","enum":["red","blue"]}}`))
	r.Register(shapedTarget("fake_b", `{"color":{"type":"string","enum":["blue","none"]}}`))
	r.Register(fakeRouter{name: "fake_ab", targets: map[string]string{"a": "fake_a", "b": "fake_b"}})
	spec, _ := r.Spec("fake_ab")
	var schema struct {
		Properties map[string]struct {
			Enum []string `json:"enum"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(spec.InputSchema, &schema); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(schema.Properties["color"].Enum, ","); got != "blue,none,red" {
		t.Errorf("color enum = %s, want every target's words", got)
	}
}

// Two targets taking a list of words advertise every word in the list's items.
func TestARoutedListCarriesEveryTargetsWords(t *testing.T) {
	r := NewRegistry(nil, nil)
	r.Register(shapedTarget("fake_a", `{"colors":{"type":"array","items":{"type":"string","enum":["red","blue"]}}}`))
	r.Register(shapedTarget("fake_b", `{"colors":{"type":"array","items":{"type":"string","enum":["blue","none"]}}}`))
	r.Register(fakeRouter{name: "fake_ab", targets: map[string]string{"a": "fake_a", "b": "fake_b"}})
	spec, _ := r.Spec("fake_ab")
	var schema struct {
		Properties map[string]struct {
			Items struct {
				Type string   `json:"type"`
				Enum []string `json:"enum"`
			} `json:"items"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(spec.InputSchema, &schema); err != nil {
		t.Fatal(err)
	}
	items := schema.Properties["colors"].Items
	if got := strings.Join(items.Enum, ","); got != "blue,none,red" || items.Type != "string" {
		t.Errorf("colors items = %+v, want strings of every target's words", items)
	}
}

// Two targets bounding one argument differently advertise the loosest bound, so
// a value one target takes is never refused by a client holding the schema.
func TestARoutedArgumentCarriesTheLoosestBound(t *testing.T) {
	r := NewRegistry(nil, nil)
	r.Register(shapedTarget("fake_a", `{"limit":{"type":"integer","minimum":1,"maximum":50},"size":{"type":"integer","maximum":9}}`))
	r.Register(shapedTarget("fake_b", `{"limit":{"type":"integer","minimum":1,"maximum":25},"size":{"type":"integer"}}`))
	r.Register(fakeRouter{name: "fake_ab", targets: map[string]string{"a": "fake_a", "b": "fake_b"}})
	spec, _ := r.Spec("fake_ab")
	var schema struct {
		Properties map[string]struct {
			Minimum *float64 `json:"minimum"`
			Maximum *float64 `json:"maximum"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(spec.InputSchema, &schema); err != nil {
		t.Fatal(err)
	}
	if limit := schema.Properties["limit"]; limit.Maximum == nil || *limit.Maximum != 50 || limit.Minimum == nil || *limit.Minimum != 1 {
		t.Errorf("limit is advertised as %+v, want 1..50", limit)
	}
	if size := schema.Properties["size"]; size.Maximum != nil {
		t.Errorf("size is advertised with maximum %v, want none: one target takes any size", *size.Maximum)
	}
}

// The loosest bound would promise a value a tighter action refuses, so the
// description names the actions that stop lower.
func TestARoutedBoundNamesTheActionsThatStopLower(t *testing.T) {
	r := NewRegistry(nil, nil)
	r.Register(shapedTarget("fake_a", `{"limit":{"type":"integer","maximum":50},"size":{"type":"integer","description":"One page."}}`))
	r.Register(shapedTarget("fake_b", `{"limit":{"type":"integer","maximum":25},"size":{"type":"integer","description":"One page."}}`))
	r.Register(shapedTarget("fake_c", `{"limit":{"type":"integer","maximum":25},"size":{"type":"integer"}}`))
	r.Register(fakeRouter{name: "fake_abc", targets: map[string]string{"a": "fake_a", "b": "fake_b", "c": "fake_c"}})
	spec, _ := r.Spec("fake_abc")
	var schema struct {
		Properties map[string]struct {
			Description string `json:"description"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(spec.InputSchema, &schema); err != nil {
		t.Fatal(err)
	}
	if got := schema.Properties["limit"].Description; got != "at most 25 for b, c." {
		t.Errorf("limit = %q, want the actions that stop at 25 named", got)
	}
	if got := schema.Properties["size"].Description; got != "One page." {
		t.Errorf("size = %q, want no note: no target bounds it", got)
	}
}

// An action that sets no bound takes any value. Every bounded action is then
// the tighter one and is named, at either end of the range.
func TestARoutedBoundNamesEveryBoundedActionWhenOneIsUnbounded(t *testing.T) {
	r := NewRegistry(nil, nil)
	r.Register(shapedTarget("fake_a", `{"n":{"type":"integer","minimum":1,"maximum":50}}`))
	r.Register(shapedTarget("fake_b", `{"n":{"type":"integer","minimum":5,"maximum":25}}`))
	r.Register(shapedTarget("fake_c", `{"n":{"type":"integer"}}`))
	r.Register(fakeRouter{name: "fake_abc", targets: map[string]string{"a": "fake_a", "b": "fake_b", "c": "fake_c"}})
	spec, _ := r.Spec("fake_abc")
	var schema struct {
		Properties map[string]struct {
			Description string `json:"description"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(spec.InputSchema, &schema); err != nil {
		t.Fatal(err)
	}
	want := "at most 25 for b; at most 50 for a; at least 5 for b; at least 1 for a."
	if got := schema.Properties["n"].Description; got != want {
		t.Errorf("n = %q, want %q", got, want)
	}
}

func TestRegisterRefusesARouterThatRestatesOrConflicts(t *testing.T) {
	for why, tc := range map[string]struct{ a, b, own string }{
		"two shapes for one argument":  {`{"x":{"type":"integer"}}`, `{"x":{"type":"string"}}`, ``},
		"two shapes for one list item": {`{"x":{"type":"array","items":{"type":"integer"}}}`, `{"x":{"type":"array","items":{"type":"string"}}}`, ``},
		"a router restating a shape":   {`{"x":{"type":"integer"}}`, `{}`, `{"x":{"type":"integer","description":"d"}}`},
		"a router naming no argument":  {`{"x":{"type":"integer"}}`, `{}`, `{"y":{"description":"d"}}`},
		"a route field a target takes": {`{"action":{"type":"string"}}`, `{}`, ``},
	} {
		mustPanic(t, why, func() {
			r := NewRegistry(nil, nil)
			r.Register(shapedTarget("fake_a", tc.a))
			r.Register(shapedTarget("fake_b", tc.b))
			schema := ""
			if tc.own != "" {
				schema = `{"type":"object","properties":` + tc.own + `}`
			}
			r.Register(fakeRouter{name: "fake_ab", targets: map[string]string{"a": "fake_a", "b": "fake_b"}, schema: schema})
		})
	}
}

// An argument only some actions take says which, so a caller sending it with
// another action is told before the target refuses it.
func TestARoutedArgumentNamesTheActionsThatTakeIt(t *testing.T) {
	r := NewRegistry(nil, nil)
	r.Register(shapedTarget("fake_a", `{"x":{"type":"integer"},"y":{"type":"boolean","description":"also the old ones"}}`))
	r.Register(shapedTarget("fake_b", `{"x":{"type":"integer"}}`))
	r.Register(fakeRouter{name: "fake_ab", targets: map[string]string{"a": "fake_a", "b": "fake_b"}})
	spec, _ := r.Spec("fake_ab")
	var schema struct {
		Properties map[string]struct {
			Description string `json:"description"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(spec.InputSchema, &schema); err != nil {
		t.Fatal(err)
	}
	if got := schema.Properties["y"].Description; got != "a only: also the old ones" {
		t.Errorf("y = %q, want it to name the one action that takes it", got)
	}
	if got := schema.Properties["x"].Description; got != "" {
		t.Errorf("x = %q, want no action named for an argument every action takes", got)
	}
}

// A colleague's agent is offered a router's waiting action as one that runs. The
// router itself runs at once when all its actions now agree.
func TestARoutersActionsAreOfferedAtTheTierAColleaguesAgentIsAdmittedOn(t *testing.T) {
	r := NewRegistry(&recordingStageApprovals{}, auth.NewGate(fullSeatAuthority{}))
	r.Register(&argsTool{spec: targetSpec("fake_a", principal.ScopeWrite, mcp.TierAutoExecute)})
	r.Register(unlistedStaging{&stagingTool{target: ids.NewV7()}})
	r.Register(fakeRouter{name: "mixed", targets: map[string]string{"a": "fake_a", "send": "send_it"}})

	var mixed mcp.ToolSpec
	for _, spec := range r.Offered(passportCtx()) {
		if spec.Name == "mixed" {
			mixed = spec
		}
	}
	if mixed.Tier != mcp.TierAutoExecute {
		t.Errorf("the router is offered at tier %v, want auto-execute", mixed.Tier)
	}
	for _, action := range mixed.Actions {
		if action.Tier != mcp.TierAutoExecute {
			t.Errorf("action %s is offered at tier %v, want auto-execute", action.Action, action.Tier)
		}
	}
	if len(mixed.Actions) != 2 {
		t.Fatalf("the router offers %d actions, want 2", len(mixed.Actions))
	}
}

// A router names only its actions that run at once and cannot be undone. A
// client asks the user before those, and not before the others.
func TestARouterAsksTheUserFirstOnlyForActionsItCannotUndo(t *testing.T) {
	r := NewRegistry(&recordingStageApprovals{}, auth.NewGate(fullSeatAuthority{}))
	archive := targetSpec("fake_archive", principal.ScopeWrite, mcp.TierAutoExecute)
	archive.AskUserFirst = true
	r.Register(&argsTool{spec: archive})
	r.Register(&argsTool{spec: targetSpec("fake_rename", principal.ScopeWrite, mcp.TierAutoExecute)})
	// A human already approves this call, so the client is not told to ask.
	merge := targetSpec("fake_merge", principal.ScopeWrite, mcp.TierConfirmationRequired)
	merge.AskUserFirst = true
	r.Register(&argsTool{spec: merge})
	r.Register(fakeRouter{name: "change", targets: map[string]string{
		"archive": "fake_archive", "rename": "fake_rename", "merge": "fake_merge",
	}})

	spec, ok := r.Spec("change")
	if !ok {
		t.Fatal("the router is not registered")
	}
	described := DescribeForClient(spec)
	if !strings.Contains(described, "the action archive cannot be undone") {
		t.Errorf("the router does not ask first for archive: %s", described)
	}
	if strings.Contains(described, "the actions ") {
		t.Errorf("the router asks first for rename, which can be undone: %s", described)
	}
	if strings.Contains(described, "merge cannot be undone") {
		t.Errorf("the router asks first for merge, which a human already approves: %s", described)
	}
}
