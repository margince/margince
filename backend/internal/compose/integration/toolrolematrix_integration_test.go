// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

const (
	toolRolesFile = "../../../../docs/reference/mcp-tool-roles.json"
	// The ids a sweep names exist nowhere, so a call reaches its gate and changes nothing.
	placeholderUUID = "00000000-0000-4000-8000-000000000001"
	toolRoleUser    = "tool-role-matrix@fable.test"
	// toolRoleCallsPerPassport stays under a passport's per-window call budget.
	toolRoleCallsPerPassport = 80
)

// TestTheToolRoleMatrixIsPublished measures, for every seeded role, which tools
// a passport on that role is refused, and holds docs/reference/mcp-tool-roles.json
// to the answer. The roles come from the database and the tools from the registry,
// so neither is a second list. A cell holds what the gate says, not what the seed
// document claims.
func TestTheToolRoleMatrixIsPublished(t *testing.T) {
	d := newTwoDoors(t, "tool-role-matrix", "read", "write", "send", "draft", "enrich")
	d.e.DescribeCompany(t)
	calls := toolRoleCalls(compose.NewRegistry(nil, compose.SendPath{}).Specs(), routeShapes())
	if len(calls) == 0 {
		t.Fatal("the registry serves no tool, so this measured nothing")
	}
	roles := seededRoles(t, d.e)
	if len(roles) == 0 {
		t.Fatal("the database seeds no role, so this measured nothing")
	}

	verdicts := map[string]toolRoleVerdicts{}
	for _, role := range roles {
		assignRoleTo(t, d.e, toolRoleUser, "admin")
		clients := passportClients(t, d.e, role.key, 1+len(calls)/toolRoleCallsPerPassport)
		pool := seedToolRoleRecords(t, d.e, calls)
		assignRoleTo(t, d.e, toolRoleUser, role.key)
		for n, call := range calls {
			got := clients[n/toolRoleCallsPerPassport].Call(t, call.verb, call.args(pool))
			row := verdicts[call.row]
			if got.IsError && strings.Contains(got.Text, "human-only operation") {
				t.Errorf("%s is offered to agents and its route refuses every agent: %s", call.verb, got.Text)
			}
			switch {
			case got.IsError && isBudgetRefusal(got.Text):
				t.Fatalf("%s as %s spent the call budget, so the rest would read as measured: %s", call.verb, role.key, got.Text)
			case !got.IsError:
				row.Allowed = append(row.Allowed, role.key)
			case strings.Contains(got.Text, "permission denied"):
				row.Refused = append(row.Refused, role.key)
			default:
				row.Unproven = append(row.Unproven, role.key)
			}
			verdicts[call.row] = row
		}
	}
	assignRoleTo(t, d.e, toolRoleUser, "admin")

	holdTheMeasurementHasTeeth(t, roles, len(calls), verdicts)
	rendered, err := json.MarshalIndent(toolRolesDocument(roles, calls, verdicts), "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	rendered = append(rendered, '\n')
	if os.Getenv("UPDATE_MCP_TOOL_ROLES") != "" {
		if err := os.WriteFile(toolRolesFile, rendered, 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	committed, err := os.ReadFile(toolRolesFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(committed) != string(rendered) {
		t.Errorf("%s is not what the roles are refused. Run: UPDATE_MCP_TOOL_ROLES=1 make test-it DIR=backend/internal/compose/integration RUN=TestTheToolRoleMatrixIsPublished, then the page's own update in internal/compose", toolRolesFile)
	}
}

// holdTheMeasurementHasTeeth fails a sweep whose calls never reach a gate: such
// a sweep would publish no role as refused anything.
func holdTheMeasurementHasTeeth(t *testing.T, roles []seededRole, calls int, verdicts map[string]toolRoleVerdicts) {
	t.Helper()
	perRole := map[string]int{}
	for _, row := range verdicts {
		for _, key := range row.Refused {
			perRole[key]++
		}
	}
	for _, role := range roles {
		if role.key != "admin" && perRole[role.key] == 0 {
			t.Errorf("role %s is refused none of %d calls, so the sweep did not reach the gates", role.key, calls)
		}
	}
}

func isBudgetRefusal(text string) bool {
	return strings.HasPrefix(text, "Too many") || strings.Contains(text, "call ceiling") || strings.Contains(text, "ceiling")
}

func passportClients(t *testing.T, e *apptest.AppEnv, role string, n int) []*apptest.MCPClient {
	t.Helper()
	clients := make([]*apptest.MCPClient, n)
	for i := range clients {
		bearer, _ := passportWithID(t, e, fmt.Sprintf("tool-role %s %d", role, i), "read", "write", "send", "draft", "enrich")
		clients[i] = apptest.NewMCPClient(e, strings.TrimPrefix(bearer["Authorization"], "Bearer "))
	}
	return clients
}

// toolRoleRoute is the part of a route verb's route a call fills in.
type toolRoleRoute struct {
	Body    json.RawMessage
	Pattern string
}

// toolRoleCall is one call the sweep makes: a verb, and the published row it is.
type toolRoleCall struct {
	row, tool, action, verb string
	input                   json.RawMessage
	route                   toolRoleRoute
}

// routeShapes is each route verb's body schema and path by verb.
func routeShapes() map[string]toolRoleRoute {
	shapes := map[string]toolRoleRoute{}
	for _, verb := range compose.RouteVerbs() {
		shapes[verb.Verb] = toolRoleRoute{Body: verb.BodySchema, Pattern: verb.Pattern}
	}
	return shapes
}

// toolRoleCalls is one call per listed tool, or per action of a listed router.
func toolRoleCalls(specs []mcp.ToolSpec, shapes map[string]toolRoleRoute) []toolRoleCall {
	byName := map[string]mcp.ToolSpec{}
	for _, spec := range specs {
		byName[spec.Name] = spec
	}
	var calls []toolRoleCall
	for _, spec := range specs {
		if spec.Unlisted {
			continue
		}
		if len(spec.Actions) == 0 {
			calls = append(calls, toolRoleCall{row: spec.Name, tool: spec.Name, verb: spec.Name, input: spec.InputSchema})
			continue
		}
		for _, action := range spec.Actions {
			calls = append(calls, toolRoleCall{
				row: spec.Name + "." + action.Action, tool: spec.Name, action: action.Action, verb: action.Verb,
				input: byName[action.Verb].InputSchema, route: shapes[action.Verb],
			})
		}
	}
	slices.SortFunc(calls, func(a, b toolRoleCall) int { return strings.Compare(a.row, b.row) })
	return calls
}

// recordPool hands each call a fresh record the sweep's user owns. So a gate
// behind a lookup is met, and one call's archive never reads as the next call's
// refusal. Counting mode answers placeholders, to learn how many to seed.
type recordPool struct {
	fresh  map[string][]string
	wanted map[string]int
}

func (p *recordPool) take(kind string) string {
	if p.wanted != nil {
		p.wanted[kind]++
		return placeholderUUID
	}
	ids := p.fresh[kind]
	p.fresh[kind] = ids[1:]
	return ids[0]
}

// recordKindOf names the record a sweep can seed for an id argument, or for the
// path segment before a route's {id}.
func recordKindOf(name string) string {
	switch name {
	case "company_id", "entity_id", "record_id", "companies":
		return "company"
	case "contact_id", "contacts":
		return "contact"
	case "deal_id", "deals":
		return "deal"
	case "lead_id", "leads":
		return "lead"
	case "activity_id", "activities":
		return "activity"
	case "attachment_id", "attachments":
		return "attachment"
	}
	return ""
}

func pathRecordKind(pattern string) string {
	before, _, found := strings.Cut(pattern, "/{")
	if !found {
		return ""
	}
	return recordKindOf(before[strings.LastIndex(before, "/")+1:])
}

func (c toolRoleCall) args(pool *recordPool) map[string]any {
	args := fillSchema(c.input, c.route.Pattern, pool)
	if _, takesBody := args["request_body"]; takesBody && len(c.route.Body) > 0 {
		args["request_body"] = fillSchema(c.route.Body, "", pool)
	}
	return args
}

// fillSchema fills a schema's required arguments, so the call passes the schema
// and meets the gate behind it.
//
//craft:ignore naked-any a JSON schema is walked as decoded JSON, whatever its shape
func fillSchema(raw json.RawMessage, pattern string, pool *recordPool) map[string]any {
	var shape struct {
		Required   []string                   `json:"required"`
		Properties map[string]json.RawMessage `json:"properties"`
	}
	args := map[string]any{}
	if err := json.Unmarshal(raw, &shape); err != nil {
		return args
	}
	for _, name := range shape.Required {
		args[name] = fillValue(name, shape.Properties[name], pattern, pool)
	}
	if _, takesBody := shape.Properties["request_body"]; takesBody {
		args["request_body"] = map[string]any{}
	}
	return args
}

//craft:ignore naked-any a JSON schema's value takes whichever type the schema names
func fillValue(name string, raw json.RawMessage, pattern string, pool *recordPool) any {
	if name == "id" {
		if kind := pathRecordKind(pattern); kind != "" {
			return pool.take(kind)
		}
	}
	if kind := recordKindOf(name); kind != "" {
		return pool.take(kind)
	}
	if name == "entity_type" || name == "record_type" {
		return "company"
	}
	var prop struct {
		Type any   `json:"type"`
		Enum []any `json:"enum"`
	}
	if err := json.Unmarshal(raw, &prop); err != nil {
		return placeholderUUID
	}
	if len(prop.Enum) > 0 {
		return prop.Enum[0]
	}
	switch prop.Type {
	case "integer", "number":
		return 1
	case "boolean":
		return false
	case "array":
		return []any{}
	case "object":
		return fillSchema(raw, "", pool)
	}
	return placeholderUUID
}

// seedToolRoleRecords creates what these calls will take, as the admin they all
// belong to, through the app's own routes.
func seedToolRoleRecords(t *testing.T, e *apptest.AppEnv, calls []toolRoleCall) *recordPool {
	t.Helper()
	counting := &recordPool{wanted: map[string]int{}}
	for _, call := range calls {
		call.args(counting)
	}
	seeded := apptest.DiscoverSeededPipeline(t, e)
	company := func() string {
		return createdID(t, e, "/v1/companies", AnyMap{"display_name": "Role Sweep " + ids.NewV7().String(), "source": "manual"})
	}
	create := map[string]func() string{
		"company": company,
		"deal": func() string {
			return createdID(t, e, "/v1/deals", AnyMap{
				"name": "Role Sweep", "amount_minor": 100, "currency": "EUR", "pipeline_id": seeded.PipelineID,
				"stage_id": seeded.Open, "company_id": company(), "source": "manual",
			})
		},
		"contact": func() string {
			return createdID(t, e, "/v1/contacts", AnyMap{"full_name": "Role Sweep", "source": "manual"})
		},
		"lead": func() string {
			return createdID(t, e, "/v1/leads", AnyMap{"full_name": "Role Sweep", "email": ids.NewV7().String() + "@lead.test", "source": "manual"})
		},
		"activity": func() string {
			return createdID(t, e, "/v1/activities", AnyMap{"kind": "task", "subject": "Role sweep"})
		},
	}
	create["attachment"] = func() string {
		status, raw := uploadAs(t, e, nil, "company", company(), []byte("PDF"))
		if status != http.StatusCreated {
			t.Fatalf("seeding an attachment → %d %s", status, raw)
		}
		return attachmentIDOf(t, raw)
	}
	pool := &recordPool{fresh: map[string][]string{}}
	for kind, n := range counting.wanted {
		for range n {
			pool.fresh[kind] = append(pool.fresh[kind], create[kind]())
		}
	}
	return pool
}

// toolRoleVerdicts sorts the roles by what one call answered them. Unproven is
// a refusal for some other reason, such as an invalid body, before any role gate.
type toolRoleVerdicts struct {
	Allowed  []string `json:"allowed_for"`
	Refused  []string `json:"refused_for"`
	Unproven []string `json:"unproven_for"`
}

type toolRoleRow struct {
	Tool   string `json:"tool"`
	Action string `json:"action,omitempty"`
	Verb   string `json:"verb"`
	toolRoleVerdicts
}

type toolRolesFileShape struct {
	Note  string        `json:"note"`
	Roles []string      `json:"roles"`
	Rows  []toolRoleRow `json:"rows"`
}

func toolRolesDocument(roles []seededRole, calls []toolRoleCall, verdicts map[string]toolRoleVerdicts) toolRolesFileShape {
	doc := toolRolesFileShape{
		Note: "Generated by TestTheToolRoleMatrixIsPublished (backend/internal/compose/integration): a passport on each seeded role calls every tool on records that do not exist. Do not edit by hand.",
	}
	for _, role := range roles {
		doc.Roles = append(doc.Roles, role.key)
	}
	for _, call := range calls {
		row := verdicts[call.row]
		// A refusal the admin meets too is about this call's record, such as a
		// relation the sweep's user lacks. It says nothing of any role.
		if slices.Contains(row.Refused, "admin") {
			row.Unproven, row.Refused = append(row.Unproven, row.Refused...), nil
		}
		published := toolRoleRow{Tool: call.tool, Action: call.action, Verb: call.verb}
		published.Allowed, published.Refused, published.Unproven = sortedRoles(row.Allowed), sortedRoles(row.Refused), sortedRoles(row.Unproven)
		doc.Rows = append(doc.Rows, published)
	}
	return doc
}

func sortedRoles(keys []string) []string {
	sorted := append([]string{}, keys...)
	slices.Sort(sorted)
	return sorted
}
