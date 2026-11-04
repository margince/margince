// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"encoding/json"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestAHumanOnlyOperationASignedInUserReachesMustSayWhy(t *testing.T) {
	_, defects := policiesOf(t, `
/record-grants:
  post:
    operationId: createRecordGrant
    security: [ { cookieAuth: [] } ]
    x-agent-access: human-only
/public/forms/{form_key}:
  post:
    operationId: submitWebForm
    security: []
    x-agent-access: human-only
`)
	if len(defects) != 1 || !strings.Contains(defects[0], "createRecordGrant") ||
		!strings.Contains(defects[0], "x-human-only-reason") {
		t.Fatalf("defects = %v, want one naming createRecordGrant's missing reason", defects)
	}
}

func TestAReasonOnlyDescribesAHumanOnlyOperation(t *testing.T) {
	_, defects := policiesOf(t, `
/deals:
  get:
    operationId: listDeals
    x-mcp-tool: { verb: list_records, tier: auto_execute, scope: read }
    x-human-only-reason: pending
`)
	if len(defects) != 1 || !strings.Contains(defects[0], "not x-agent-access: human-only") {
		t.Fatalf("defects = %v, want the stray reason refused", defects)
	}
}

func TestTheReasonIsAClosedVocabulary(t *testing.T) {
	policies, derived := policiesOf(t, `
/record-grants:
  post:
    operationId: createRecordGrant
    x-agent-access: human-only
    x-human-only-reason: later
`)
	if len(derived) != 0 {
		t.Fatalf("derivation refused the value itself: %v", derived)
	}
	vocab := vocabulary{access: []string{"human-only"}, reason: []string{"security", "covered", "pending"}}
	defects := vocab.violations(policies)
	if len(defects) != 1 || !strings.Contains(defects[0], `x-human-only-reason = "later"`) {
		t.Fatalf("defects = %v, want the undeclared reason refused", defects)
	}
}

func routeRow(router, action, tier, scope string) string {
	return "    x-mcp-tool: { verb: v_" + action + ", router: " + router + ", action: " + action +
		", tier: " + tier + ", scope: " + scope + ", served_by: route }\n"
}

func TestRouteBackedRowsTheDoorCannotServeFailGeneration(t *testing.T) {
	for name, tc := range map[string]struct{ paths, want string }{
		"router without served_by: route": {
			"/a:\n  post:\n    operationId: a\n    x-mcp-tool: { verb: v, router: change_access, action: share, tier: auto_execute, scope: write }\n",
			"belong to served_by: route",
		},
		"served_by: route without a router": {
			"/a:\n  post:\n    operationId: a\n    x-mcp-tool: { verb: v, tier: auto_execute, scope: write, served_by: route }\n",
			"belong to served_by: route",
		},
		"action without router": {
			"/a:\n  post:\n    operationId: a\n    x-mcp-tool: { verb: v, action: share, tier: auto_execute, scope: write, served_by: route }\n",
			"a router and an action together",
		},
		"a confirm tier": {
			"/a:\n  post:\n    operationId: a\n" + routeRow("change_access", "share", "confirmation_required", "write"),
			"must be auto_execute",
		},
		"the enrich cap": {
			"/a:\n  post:\n    operationId: a\n" + routeRow("change_access", "share", "auto_execute", "enrich"),
			"a route-backed enrich belongs to run_research",
		},
		"a send outside send_outbound": {
			"/a:\n  post:\n    operationId: a\n" + routeRow("change_offers", "invite", "auto_execute", "send"),
			"belongs to send_outbound",
		},
		"one action twice": {
			"/a:\n  post:\n    operationId: a\n" + routeRow("change_access", "share", "auto_execute", "write") +
				"/b:\n  post:\n    operationId: b\n" + strings.Replace(routeRow("change_access", "share", "auto_execute", "write"), "v_share", "v_other", 1),
			"is already",
		},
		"two scopes in one router": {
			"/a:\n  post:\n    operationId: a\n" + routeRow("change_access", "share", "auto_execute", "write") +
				"/b:\n  get:\n    operationId: b\n" + routeRow("change_access", "list", "auto_execute", "read"),
			"one router, one scope",
		},
	} {
		t.Run(name, func(t *testing.T) {
			policies, defects := policiesOf(t, tc.paths)
			defects = append(defects, routeBackedDefects(policies, testRouters)...)
			if len(defects) != 1 || !strings.Contains(defects[0], tc.want) {
				t.Fatalf("defects = %v, want one containing %q", defects, tc.want)
			}
		})
	}
}

func TestAnInviteMailIsARouteBackedSendOnSendOutbound(t *testing.T) {
	policies, defects := policiesOf(t, "/a:\n  post:\n    operationId: a\n"+
		routeRow("send_outbound", "deal_room_invite", "auto_execute", "send"))
	defects = append(defects, routeBackedDefects(policies, testRouters)...)
	if len(defects) != 0 {
		t.Fatalf("defects = %v, want none", defects)
	}
	table := renderTable(policies, vocabulary{})
	if !strings.Contains(table, `ServedBy: "route", Router: "send_outbound", Action: "deal_room_invite"`) {
		t.Errorf("the emitted row does not carry its router and action:\n%s", table)
	}
}

func TestARouteBackedUploadFailsGeneration(t *testing.T) {
	defects := nonJSONBodies(map[string]routeShape{
		"POST /v1/company/logo":     {body: true},
		"POST /v1/record-grants":    {body: true, bodySchema: `{"type":"object"}`},
		"POST /v1/offers/{id}/send": {},
	})
	if len(defects) != 1 || !strings.Contains(defects[0], "/v1/company/logo") {
		t.Fatalf("defects = %v, want the upload alone", defects)
	}
}

func TestABodySchemaReachesTheAgentAsOnePlainShape(t *testing.T) {
	var node yaml.Node
	if err := yaml.Unmarshal([]byte(`
content:
  application/json:
    schema:
      allOf:
        - $ref: '#/components/schemas/Base'
        - type: object
          required: [note]
          properties:
            note: { type: string, description: words a caller does not need }
            label: { type: [string, 'null'] }
            when:
              oneOf:
                - type: 'null'
                - { type: string, format: date }
`), &node); err != nil {
		t.Fatal(err)
	}
	schemas := map[string]any{"Base": map[string]any{
		"type": "object", "required": []any{"id"}, "properties": map[string]any{"id": map[string]any{"type": "string"}},
	}}
	got, err := bodySchema(node.Content[0], schemas)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"id":{"type":"string"}`, `"note":{"type":"string"}`, `"label":{"type":"string"}`, `"when":{"format":"date","type":"string"}`, `"required":["id","note"]`} {
		if !strings.Contains(got, want) {
			t.Errorf("body schema %s lacks %s", got, want)
		}
	}
	for _, refused := range []string{"allOf", "oneOf", "description", "$ref"} {
		if strings.Contains(got, refused) {
			t.Errorf("body schema %s still carries %s", got, refused)
		}
	}
}

func TestAnAnswerSchemaIsWhatTheDoorAnswers(t *testing.T) {
	decode := func(src string) yaml.Node {
		var node yaml.Node
		if err := yaml.Unmarshal([]byte(src), &node); err != nil {
			t.Fatal(err)
		}
		return *node.Content[0]
	}
	list, err := answerSchema(decode(`
'200':
  content:
    application/json:
      schema: { type: array, items: { type: string } }
`), nil)
	if err != nil || list != `{"properties":{"items":{"items":{"type":"string"},"type":"array"}},"type":"object"}` {
		t.Errorf("an array answer = %s, %v; want it wrapped as items", list, err)
	}
	none, err := answerSchema(decode(`'204': { description: Done. }`), nil)
	if err != nil || !strings.Contains(none, `"status"`) {
		t.Errorf("a body-less answer = %s, %v; want the status shape", none, err)
	}
}

// testRouters are the routers the cases below name.
var testRouters = map[string]bool{"change_state": true, "change_access": true, "send_outbound": true, "run_research": true, "read_details": true, "change_offers": true}

func TestAnActionOnARouterNobodyRegistersIsRefused(t *testing.T) {
	policies, derived := policiesOf(t, `
/deals/{id}/revert:
  post:
    operationId: revertDeal
    x-mcp-tool: { verb: revert_deal, router: change_stat, action: revert, tier: auto_execute, scope: write, served_by: route }
`)
	derived = append(derived, routeBackedDefects(policies, testRouters)...)
	if len(derived) == 0 || !strings.Contains(strings.Join(derived, ";"), "router change_stat is no router") {
		t.Fatalf("defects = %v, want the unknown router refused", derived)
	}
}

// The real package, so a renamed router or a moved Name fails here.
func TestTheRoutersAreReadFromTheAgentsPackage(t *testing.T) {
	routers, err := registeredRouters("../../internal/modules/agents")
	if err != nil {
		t.Fatal(err)
	}
	for _, router := range []string{"change_access", "read_workspace", "send_outbound", "change_state", "change_tags", "list_documents", "read_reports"} {
		if !routers[router] {
			t.Errorf("%s is not among the routers read: %v", router, routers)
		}
	}
}

func TestAFieldNamedLikeProseKeepsItsPlaceInABodySchema(t *testing.T) {
	out, err := resolveRefs(map[string]any{
		"type":        "object",
		"description": "dropped",
		"properties": map[string]any{
			"description": map[string]any{"type": "string", "description": "dropped too"},
			"example":     map[string]any{"type": "string"},
		},
	}, nil, 0, map[string]bool{})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"properties":{"description":{"type":"string"},"example":{"type":"string"}},"type":"object"}`
	if string(encoded) != want {
		t.Fatalf("schema = %s, want %s", encoded, want)
	}
}

func TestAQueryParameterKeepsWhatTheRouteRequiresOfIt(t *testing.T) {
	var param queryParameter
	if err := yaml.Unmarshal([]byte(`
name: entity_type
in: query
required: true
schema: { $ref: '#/components/schemas/Kind' }
`), &param); err != nil {
		t.Fatal(err)
	}
	schemas := map[string]schemaNode{"Kind": {Type: "string", Enum: []string{"company", "deal"}}}
	got, err := queryOf(param, schemas)
	if err != nil {
		t.Fatal(err)
	}
	if want := `{Name: "entity_type", Type: "string", Required: true, Enum: []string{"company", "deal"}}`; got.literal() != want {
		t.Errorf("the query parameter renders as %s, want %s", got.literal(), want)
	}
	param.Required, param.Schema.Ref, param.Schema.Type, param.Schema.Format = false, "", "string", "uuid"
	if got, _ := queryOf(param, schemas); got.literal() != `{Name: "entity_type", Type: "string", Format: "uuid"}` {
		t.Errorf("an optional uuid renders as %s", got.literal())
	}
}

func TestAnAnswerAdmitsTheNullItsContractAdmits(t *testing.T) {
	var node yaml.Node
	if err := yaml.Unmarshal([]byte(`
'200':
  content:
    application/schema+json:
      schema:
        type: object
        required: [budget, version, tier, months]
        properties:
          budget: { type: [integer, 'null'] }
          version: { type: integer, enum: [1] }
          tier: { type: [string, 'null'], enum: [gold, null] }
          months: { type: integer }
`), &node); err != nil {
		t.Fatal(err)
	}
	got, err := answerSchema(*node.Content[0], nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"required":["version","months"]`, `"tier":{"enum":["gold"],"type":"string"}`, `"version":{"type":"integer"}`} {
		if !strings.Contains(got, want) {
			t.Errorf("answer schema %s lacks %s", got, want)
		}
	}
	if strings.Contains(got, nullableMarker) || strings.Contains(got, "size_bytes") {
		t.Errorf("answer schema %s carries a marker or reads a JSON document as a file", got)
	}
}

func TestAClosedAnswerAdmitsTheReadableFigureBesideAnAmount(t *testing.T) {
	var node yaml.Node
	if err := yaml.Unmarshal([]byte(`
'200':
  content:
    application/json:
      schema:
        type: object
        additionalProperties: false
        properties:
          currency: { type: string }
          total_minor: { type: integer }
`), &node); err != nil {
		t.Fatal(err)
	}
	got, err := answerSchema(*node.Content[0], nil)
	if err != nil || !strings.Contains(got, `"total_display":{"type":"string"}`) {
		t.Errorf("answer schema %s (%v) does not admit total_display", got, err)
	}
}
