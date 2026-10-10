// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// agentGate leaves a route marked `served_by: registry` to Registry.Invoke.
// The mark is safe only while the handler calls Invoke with the tool the
// contract names. A handler that skipped it would serve an agent with no
// admission at all. Both tests derive their routes from the policy table, so a
// route marked tomorrow is held to the same promise.

import (
	"context"
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/httpserver"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// registryServedRoutes answers the marked routes, sorted.
func registryServedRoutes(t *testing.T) []string {
	t.Helper()
	var routes []string
	for route, pol := range agentPolicies {
		if pol.ServedBy == servedByRegistry {
			routes = append(routes, route)
		}
	}
	if len(routes) == 0 {
		t.Fatal("no route is marked served_by: registry, so this gate checked nothing")
	}
	sort.Strings(routes)
	return routes
}

// recordingTool stands in for the tool a route declares and counts its runs.
// Only Invoke reaches Handle, so a run proves the handler went through it.
type recordingTool struct {
	name string
	ran  map[string]int
}

func (t recordingTool) Spec() mcp.ToolSpec {
	return mcp.ToolSpec{
		Name: t.name, Title: t.name, Version: "1", Description: "stands in for " + t.name,
		RequiredScope: principal.ScopeRead, Tier: mcp.TierAutoExecute,
		InputSchema: json.RawMessage(`{"type":"object"}`),
	}
}

func (t recordingTool) Handle(context.Context, json.RawMessage) (json.RawMessage, error) {
	t.ran[t.name]++
	return json.RawMessage(`{}`), nil
}

func TestEveryRegistryServedRouteRunsTheToolItDeclaresThroughInvoke(t *testing.T) {
	routes := registryServedRoutes(t)
	ran := map[string]int{}
	gate := auth.NewGate(fullSeat{})
	registry := agents.NewRegistry(nil, gate)
	registered := map[string]bool{}
	for _, route := range routes {
		if tool := agentPolicies[route].Tool; !registered[tool] {
			registry.Register(recordingTool{name: tool, ran: ran})
			registered[tool] = true
		}
	}
	router := crmcontracts.HandlerWithOptions(Server{toolRegistry: registry}, crmcontracts.ChiServerOptions{
		BaseURL:     httpserver.BaseURL,
		Middlewares: []crmcontracts.MiddlewareFunc{agentGate(registry, nil, nil, nil, nil, nil, nil, gate)},
	})
	contract := operationsByID(t)

	for _, route := range routes {
		pol := agentPolicies[route]
		before := ran[pol.Tool]
		allowed := serveAs(agentHolding(t.Context(), principal.ScopeRead), t, router, route, contract[pol.Op])
		if allowed.Code != http.StatusOK || ran[pol.Tool] != before+1 {
			t.Errorf("%s (%s) answered %d and ran %s %d times, want 200 after one run: its handler must "+
				"call Registry.Invoke with the tool its x-mcp-tool names, since agentGate admits nothing "+
				"on it", route, pol.Op, allowed.Code, pol.Tool, ran[pol.Tool]-before)
		}
		refused := serveAs(agentHolding(t.Context()), t, router, route, contract[pol.Op])
		if refused.Code != http.StatusForbidden || ran[pol.Tool] != before+1 {
			t.Errorf("%s (%s) answered %d to a passport without the read scope, want 403 from Invoke's "+
				"admission", route, pol.Op, refused.Code)
		}
	}
}

func agentHolding(ctx context.Context, scopes ...principal.Scope) context.Context {
	ctx = principal.WithWorkspaceID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalAgent, ID: "agent:route-guard", OnBehalfOf: ids.NewV7(), PassportID: ids.NewV7(),
		Scopes: principal.NewScopeSet(scopes...),
	})
}

// serveAs sends route a request the contract accepts: every required
// parameter filled, and an empty JSON object for a body.
func serveAs(ctx context.Context, t *testing.T, router http.Handler, route string, op *openapi3.Operation) *httptest.ResponseRecorder {
	t.Helper()
	if op == nil {
		t.Fatalf("%s names an operation crm.yaml does not declare", route)
	}
	method, path, _ := strings.Cut(route, " ")
	query := url.Values{}
	for _, ref := range op.Parameters {
		param := ref.Value
		value := placeholderFor(param.Schema.Value)
		if param.Schema.Value.Format == "uuid" {
			value = ids.NewV7().String()
		}
		switch {
		case param.In == openapi3.ParameterInPath:
			path = strings.ReplaceAll(path, "{"+param.Name+"}", stringOf(value))
		case param.In == openapi3.ParameterInQuery && param.Required:
			query.Set(param.Name, stringOf(value))
		}
	}
	body := ""
	if op.RequestBody != nil {
		body = `{}`
	}
	req := httptest.NewRequest(method, path+"?"+query.Encode(), strings.NewReader(body)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	return recorder
}

//craft:ignore naked-any the placeholder is whichever JSON scalar the schema declares
func stringOf(value any) string {
	if s, ok := value.(string); ok {
		return s
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(raw)
}

// operationsByID indexes the contract's operations, with each path's shared
// parameters folded into the operation that inherits them.
func operationsByID(t *testing.T) map[string]*openapi3.Operation {
	t.Helper()
	byID := map[string]*openapi3.Operation{}
	for _, item := range loadContract(t).Paths.Map() {
		for _, op := range item.Operations() {
			inherited := *op
			inherited.Parameters = append(slices.Clone(item.Parameters), op.Parameters...)
			byID[op.OperationID] = &inherited
		}
	}
	return byID
}

// A registry-served route writes the tool's payload as its 200 body, so the
// contract's 200 schema and the tool's declared output must be one shape. Two
// hand-written copies of it would otherwise drift apart.
func TestEveryRegistryServedRouteDeclaresTheShapeItsToolAnswers(t *testing.T) {
	contract := operationsByID(t)
	registry := NewRegistry(nil, SendPath{})
	for _, route := range registryServedRoutes(t) {
		pol := agentPolicies[route]
		spec, registered := registry.Spec(pol.Tool)
		if !registered {
			t.Errorf("%s declares %s, which is not registered", route, pol.Tool)
			continue
		}
		var envelope struct {
			Properties struct {
				Data toolShape `json:"data"`
			} `json:"properties"`
		}
		if err := json.Unmarshal(spec.OutputSchema, &envelope); err != nil {
			t.Fatalf("%s's output schema does not decode: %v", pol.Tool, err)
		}
		ok := contract[pol.Op].Responses.Status(http.StatusOK)
		if ok == nil || ok.Value.Content.Get("application/json") == nil {
			t.Errorf("%s (%s) declares no JSON 200 body", route, pol.Op)
			continue
		}
		declared := ok.Value.Content.Get("application/json").Schema.Value
		for _, problem := range shapeDifferences(pol.Op, declared, envelope.Properties.Data) {
			t.Errorf("%s: %s", route, problem)
		}
	}
}

// toolShape is the part of a tool's derived JSON Schema a wire shape is made of.
type toolShape struct {
	Type       string               `json:"type"`
	Properties map[string]toolShape `json:"properties"`
	Items      *toolShape           `json:"items"`
	Required   []string             `json:"required"`
}

func shapeDifferences(at string, declared *openapi3.Schema, answered toolShape) []string {
	if !declared.Type.Is(answered.Type) {
		return []string{at + " is " + strings.Join(declared.Type.Slice(), ",") + " in the contract and " +
			answered.Type + " in the tool's output"}
	}
	var problems []string
	if answered.Items != nil && declared.Items != nil {
		problems = append(problems, shapeDifferences(at+"[]", declared.Items.Value, *answered.Items)...)
	}
	names := slices.Sorted(maps.Keys(answered.Properties))
	contractNames := slices.Sorted(maps.Keys(declared.Properties))
	if !slices.Equal(names, contractNames) {
		return append(problems, at+" has members "+strings.Join(contractNames, ",")+" in the contract and "+
			strings.Join(names, ",")+" in the tool's output")
	}
	required := slices.Sorted(slices.Values(declared.Required))
	if !slices.Equal(required, slices.Sorted(slices.Values(answered.Required))) {
		problems = append(problems, at+" requires "+strings.Join(required, ",")+" in the contract and "+
			strings.Join(answered.Required, ",")+" in the tool's output")
	}
	for _, name := range names {
		problems = append(problems, shapeDifferences(at+"."+name, declared.Properties[name].Value, answered.Properties[name])...)
	}
	return problems
}
