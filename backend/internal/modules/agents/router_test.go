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
	if served.Tool != "fake_a" {
		t.Errorf("served tool = %q, want fake_a", served.Tool)
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

// The retry claim is the target's, and over the call the target sees: a key
// spent through the router and the same call made by the old name are one call.
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
	small.InputSchema = json.RawMessage(`{"type":"object","required":["x"],"properties":{"x":{"type":"integer"},"y":{"type":"string"}}}`)
	r.Register(&argsTool{spec: big})
	r.Register(&argsTool{spec: small})
	r.Register(fakeRouter{
		name: "fake_ab", targets: map[string]string{"b": "fake_b", "a": "fake_a"},
		schema: `{"type":"object","properties":{"x":{"type":"integer","description":"what x means for every action"}}}`,
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
	if err := json.Unmarshal(schema.Properties["action"], &route); err != nil || route.Type != "string" ||
		strings.Join(route.Enum, ",") != "a,b" {
		t.Errorf("route field = %s, want a string enum of the sorted choices", schema.Properties["action"])
	}
	if _, ok := schema.Properties["y"]; !ok {
		t.Error("property y of a target is missing")
	}
	if !strings.Contains(string(schema.Properties["x"]), "every action") {
		t.Errorf("x = %s, want the router's own wording over a target's", schema.Properties["x"])
	}
	if strings.Join(schema.Required, ",") != "action,x" {
		t.Errorf("required = %v, want the route field and what every target requires", schema.Required)
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
		"two tiers behind one tool":        {"a": "fake_a", "c": "fake_c"},
		"a target still offered":           {"a": "fake_a", "l": "fake_l"},
	} {
		mustPanic(t, why, func() {
			setup().Register(fakeRouter{name: "bad", targets: targets})
		})
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
