// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// A route-backed action runs its route's own handler. So a share through
// change_access leaves what the REST share leaves. Each of two equal records
// gets one grant to the same team, with the same access.
func TestARouteBackedActionAnswersAsItsRoute(t *testing.T) {
	d := newTwoDoors(t, "route-backed-doors", "read", "write")
	team := createdRecord(t, d.e, "/v1/teams", AnyMap{"name": "Doors"})
	first := createdRecord(t, d.e, "/v1/companies", AnyMap{"source": "manual", "display_name": "Door One"})
	second := createdRecord(t, d.e, "/v1/companies", AnyMap{"source": "manual", "display_name": "Door Two"})
	share := func(company string) AnyMap {
		return AnyMap{"record_type": "company", "record_id": company, "subject_type": "team", "subject_id": team, "access": "read"}
	}
	type grant struct {
		RecordType  string `json:"record_type"`
		SubjectType string `json:"subject_type"`
		SubjectID   string `json:"subject_id"`
		Access      string `json:"access"`
	}
	var viaREST json.RawMessage
	if status := d.e.Call(t, "POST", "/v1/record-grants", share(first), d.bearer, &viaREST); status != http.StatusCreated {
		t.Fatalf("REST share answered %d %s", status, viaREST)
	}
	viaTool := sameRecords[grant](t, "share", viaREST,
		d.tool(t, "change_access", map[string]any{"action": "share", "request_body": share(second)}))
	if viaTool.SubjectID != team || viaTool.Access != "read" {
		t.Errorf("the tool's grant = %+v, want team %s with read", viaTool, team)
	}
}

// Which model runs, how long data is kept, and what past AI calls held stay with
// a signed-in user. An agent is refused them on the route and has no topic for them.
func TestAnAgentIsRefusedTheOperatorReads(t *testing.T) {
	d := newTwoDoors(t, "operator-reads", "read", "write", "send", "enrich")
	for _, path := range []string{"/v1/ai/routing", "/v1/retention/settings", "/v1/ai/calls", "/v1/ai/calls/00000000-0000-4000-8000-000000000001"} {
		if status := d.e.Call(t, "GET", path, nil, d.bearer, nil); status != http.StatusForbidden {
			t.Errorf("GET %s as a passport → %d, want 403", path, status)
		}
	}
	for _, topic := range []string{"ai_routing", "retention_settings", "ai_calls", "ai_call"} {
		d.mcp.CallRefused(t, "read_workspace", map[string]any{"topic": topic})
	}
}

// AI work an agent starts spends the enrich scope, so a passport lent only read
// and write cannot start it, on any role.
func TestRunResearchNeedsTheEnrichScope(t *testing.T) {
	d := newTwoDoors(t, "run-research-scope", "read", "write")
	company := createdRecord(t, d.e, "/v1/companies", AnyMap{"source": "manual", "display_name": "Scope Works"})
	got := d.mcp.Call(t, "run_research", map[string]any{"action": "refresh_dossier", "id": company})
	if !got.IsError || !strings.Contains(got.Text, "scope") {
		t.Fatalf("run_research without enrich answered %+v, want a scope refusal", got)
	}
}

// Every route-backed action admits an agent acting for its user wherever it
// admits that user. Each is called on records that do not exist, as the
// signed-in admin over REST and as their passport over MCP. The human may be
// refused too, for a relation the placeholder lacks. An agent refused where its
// human is admitted meets a human check still on the path.
func TestNoRouteBackedActionStillRefusesAnActingAgent(t *testing.T) {
	d := newTwoDoors(t, "route-backed-sweep", "read", "write", "send", "enrich")
	verbs := compose.RouteVerbs()
	if len(verbs) == 0 {
		t.Fatal("the build serves no route-backed verb, so this swept nothing")
	}
	compared := 0
	for n, v := range verbs {
		// A passport's call budget is a per-credential window; a fresh one per
		// batch keeps the sweep under it.
		if n > 0 && n%passportCallsPerBatch == 0 {
			bearer, _ := passportWithID(t, d.e, fmt.Sprintf("route-backed-sweep %d", n), "read", "write", "send", "enrich")
			d.mcp = apptest.NewMCPClient(d.e, strings.TrimPrefix(bearer["Authorization"], "Bearer "))
		}
		args, path := map[string]any{}, v.Pattern
		for _, param := range v.PathParams {
			args[param] = placeholderFor(param)
			path = strings.Replace(path, "{"+param+"}", placeholderFor(param), 1)
		}
		var body any
		if v.Body {
			args["request_body"] = map[string]any{}
			body = map[string]any{}
		}
		human := d.e.Call(t, v.Method, path, body, nil, nil)
		result := d.mcp.Call(t, v.Verb, args)
		if human == http.StatusForbidden || human == http.StatusUnauthorized {
			continue
		}
		compared++
		if result.IsError && strings.Contains(result.Text, "permission denied") {
			t.Errorf("%s (%s %s) admits the signed-in admin (%d) and refuses their agent: %s",
				v.Verb, v.Method, v.Pattern, human, result.Text)
		}
	}
	if compared == 0 {
		t.Fatal("the signed-in admin was refused every route-backed action, so no agent answer was compared")
	}
}

// passportCallsPerBatch stays under a passport's per-window call budget.
const passportCallsPerBatch = 80

// placeholderFor is a path value naming nothing that exists, shaped as the
// parameter takes it, so the call reaches past routing to the handler.
func placeholderFor(param string) string {
	switch {
	case strings.HasSuffix(param, "_type") || strings.HasSuffix(param, "Type"):
		return "deal"
	case param == "id" || strings.HasSuffix(param, "_id") || strings.HasSuffix(param, "Id"):
		return "00000000-0000-4000-8000-000000000001"
	}
	return "none"
}
