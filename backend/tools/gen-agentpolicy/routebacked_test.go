// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"encoding/json"
	"slices"
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

func TestARouteBodyThatIsNeitherJSONNorOneFileFailsGeneration(t *testing.T) {
	defects := nonJSONBodies(map[string]routeShape{
		"POST /v1/raw":              {body: true},
		"POST /v1/company/logo":     {body: true, uploadField: "file"},
		"POST /v1/record-grants":    {body: true, bodySchema: `{"type":"object"}`},
		"POST /v1/offers/{id}/send": {},
	})
	if len(defects) != 1 || !strings.Contains(defects[0], "/v1/raw") {
		t.Fatalf("defects = %v, want the raw body alone", defects)
	}
}

func TestAnUploadRouteNamesItsOneFileField(t *testing.T) {
	cases := map[string]struct {
		body, field string
		refused     bool
	}{
		"one file": {body: `
content:
  multipart/form-data:
    schema: { type: object, properties: { file: { type: string, format: binary } } }
`, field: "file"},
		"a JSON body": {body: `
content:
  application/json:
    schema: { type: object, properties: { name: { type: string } } }
`},
		"a second field the door would drop": {body: `
content:
  multipart/form-data:
    schema: { type: object, properties: { file: { type: string, format: binary }, object: { type: string } } }
`, refused: true},
		"no file": {body: `
content:
  multipart/form-data:
    schema: { type: object, properties: { note: { type: string } } }
`, refused: true},
	}
	for name, tc := range cases {
		var doc yaml.Node
		if err := yaml.Unmarshal([]byte(tc.body), &doc); err != nil {
			t.Fatal(err)
		}
		field, err := uploadField(doc.Content[0])
		if (err != nil) != tc.refused || field != tc.field {
			t.Errorf("%s: field %q, err %v; want field %q, refused %t", name, field, err, tc.field, tc.refused)
		}
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
          description: words about the schema, which the shape does not need
          required: [note]
          properties:
            note: { type: string, description: what a caller may write here }
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
	got, _, err := bodySchema(node.Content[0], schemas)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"id":{"type":"string"}`, `"note":{"description":"what a caller may write here","type":"string"}`, `"label":{"type":"string"}`, `"when":{"format":"date","type":"string"}`, `"required":["id","note"]`} {
		if !strings.Contains(got, want) {
			t.Errorf("body schema %s lacks %s", got, want)
		}
	}
	for _, refused := range []string{"allOf", "oneOf", "about the schema", "$ref"} {
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
	out, err := (&refWalk{open: map[string]bool{}}).resolve(map[string]any{
		"type":        "object",
		"description": "dropped",
		"properties": map[string]any{
			"description": map[string]any{"type": "string", "description": "dropped too"},
			"example":     map[string]any{"type": "string"},
		},
	}, 0)
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

// A list carries its members' words, so a caller learns which values the route
// takes before the route refuses one.
func TestAListParameterCarriesItsMembersWords(t *testing.T) {
	var param queryParameter
	if err := yaml.Unmarshal([]byte(`
name: state
in: query
schema: { type: array, items: { $ref: '#/components/schemas/State' } }
style: form
explode: true
`), &param); err != nil {
		t.Fatal(err)
	}
	schemas := map[string]schemaNode{"State": {Type: "string", Enum: []string{"open", "closed"}}}
	got, err := queryOf(param, schemas)
	if err != nil {
		t.Fatal(err)
	}
	if want := `{Name: "state", Type: "array", Enum: []string{"open", "closed"}}`; got.literal() != want {
		t.Errorf("the list renders as %s, want %s", got.literal(), want)
	}
	param.Schema.Items = &parameterSet{Format: "uuid"}
	if got, _ := queryOf(param, schemas); got.literal() != `{Name: "state", Type: "array", Format: "uuid"}` {
		t.Errorf("a list of uuids renders as %s", got.literal())
	}
}

// The door sends one key per member, so a route reading a list another way
// fails generation instead of answering a request it misreads.
func TestAListTheDoorCannotSendFailsGeneration(t *testing.T) {
	for _, encoding := range []string{"style: form\nexplode: false", "style: spaceDelimited"} {
		var param queryParameter
		if err := yaml.Unmarshal([]byte("name: types\nin: query\nschema: { type: array, items: { type: string } }\n"+encoding), &param); err != nil {
			t.Fatal(err)
		}
		if _, err := queryOf(param, nil); err == nil || !strings.Contains(err.Error(), "one key per member") {
			t.Errorf("a list read as %q generated (%v), want it refused", encoding, err)
		}
	}
}

// A member an answer may hold as null, or a route may leave out, must not fail the
// check that withholds a client's structured answer. A field named required stays.
func TestAnAnswerSchemaRequiresNoNestedMember(t *testing.T) {
	var node yaml.Node
	if err := yaml.Unmarshal([]byte(`
'200':
  content:
    application/schema+json:
      schema:
        type: object
        required: [last_drill, items]
        properties:
          last_drill: { type: [string, 'null'] }
          required: { type: boolean }
          items:
            type: array
            items: { type: object, required: [id], properties: { id: { type: string } } }
`), &node); err != nil {
		t.Fatal(err)
	}
	got, err := answerSchema(*node.Content[0], map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, `"required":[`) {
		t.Errorf("the answer schema still requires a member: %s", got)
	}
	if !strings.Contains(got, `"required":{"type":"boolean"}`) || !strings.Contains(got, `"last_drill":{"type":"string"}`) {
		t.Errorf("the answer schema lost a field: %s", got)
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

func TestAClosedAnswerAdmitsTheReadableFigureUnderAnEnclosingBaseCurrency(t *testing.T) {
	var node yaml.Node
	if err := yaml.Unmarshal([]byte(`
'200':
  content:
    application/json:
      schema:
        type: object
        additionalProperties: false
        properties:
          readings:
            type: object
            additionalProperties: false
            properties:
              base_currency: { type: string }
              by_source:
                type: object
                additionalProperties: false
                properties:
                  human:
                    type: object
                    additionalProperties: false
                    properties: { open_minor: { type: integer } }
              rows:
                type: array
                items:
                  type: object
                  additionalProperties: false
                  properties: { weighted_minor: { type: integer } }
              by_region:
                type: object
                additionalProperties:
                  type: object
                  additionalProperties: false
                  properties: { region_minor: { type: integer } }
              row:
                type: object
                required: [currency]
                properties:
                  currency: { type: string }
                  child:
                    type: object
                    additionalProperties: false
                    properties: { owned_minor: { type: integer } }
          loose:
            type: object
            additionalProperties: false
            properties: { stray_minor: { type: integer } }
`), &node); err != nil {
		t.Fatal(err)
	}
	got, err := answerSchema(*node.Content[0], nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"open_display":{"type":"string"}`, `"weighted_display":{"type":"string"}`, `"region_display":{"type":"string"}`} {
		if !strings.Contains(got, want) {
			t.Errorf("answer schema %s does not admit %s under the enclosing base_currency", got, want)
		}
	}
	for _, refused := range []string{"stray_display", "owned_display"} {
		if strings.Contains(got, refused) {
			t.Errorf("answer schema %s admits %s where no currency is in scope", got, refused)
		}
	}
}

func TestADeprecatedOperationsVerbSaysSoInItsSummary(t *testing.T) {
	var paths map[string]map[string]yaml.Node
	if err := yaml.Unmarshal([]byte(`
/weeks:
  get: { summary: A team's week., deprecated: true }
/days:
  get: { summary: A rep's day. }
`), &paths); err != nil {
		t.Fatal(err)
	}
	policies := []policy{{Route: "GET /v1/weeks"}, {Route: "GET /v1/days"}}
	shapes, err := routeShapes(paths, nil, nil, nil, policies, func(policy) bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	if got := shapes["GET /v1/weeks"].summary; got != "Deprecated: A team's week." {
		t.Errorf("deprecated verb summary = %q", got)
	}
	if got := shapes["GET /v1/days"].summary; got != "A rep's day." {
		t.Errorf("supported verb summary = %q", got)
	}
}

// A member a human alone may set is left out of the shape an agent is offered.
// A body that requires one cannot be offered at all.
func TestAHumanOnlyMemberIsLeftOutOfTheAgentBody(t *testing.T) {
	body := func(required string) *yaml.Node {
		var node yaml.Node
		if err := yaml.Unmarshal([]byte(`
content:
  application/json:
    schema:
      type: object
      required: [`+required+`]
      properties:
        status: { type: string }
        kind: { type: string, x-human-only: true }
`), &node); err != nil {
			t.Fatal(err)
		}
		return node.Content[0]
	}
	got, marks, err := bodySchema(body("status"), map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, `"status"`) || strings.Contains(got, `"kind"`) || strings.Contains(got, "x-human-only") {
		t.Errorf("body schema %s, want status alone and no marker", got)
	}
	if !slices.Equal(marks.HumanOnly, []string{"kind"}) {
		t.Errorf("human-only members %v, want [kind] for the agent gate", marks.HumanOnly)
	}
	if _, _, err := bodySchema(body("kind"), map[string]any{}); err == nil || !strings.Contains(err.Error(), "kind") {
		t.Errorf("a body requiring a human-only member answered %v, want a refusal naming it", err)
	}
	var referenced yaml.Node
	if err := yaml.Unmarshal([]byte(`
content:
  application/json:
    schema:
      type: object
      properties:
        kind: { $ref: '#/components/schemas/Kind', x-human-only: true }
`), &referenced); err != nil {
		t.Fatal(err)
	}
	got, marks, err = bodySchema(referenced.Content[0], map[string]any{"Kind": map[string]any{"type": "string"}})
	if err != nil || strings.Contains(got, `"kind"`) || !slices.Equal(marks.HumanOnly, []string{"kind"}) {
		t.Errorf("a marker beside $ref: schema %s, human-only %v (%v), want kind left out and named", got, marks.HumanOnly, err)
	}
	var nested yaml.Node
	if err := yaml.Unmarshal([]byte(`
content:
  application/json:
    schema:
      type: object
      properties:
        line: { type: object, properties: { price: { type: number, x-human-only: true } } }
`), &nested); err != nil {
		t.Fatal(err)
	}
	if _, _, err := bodySchema(nested.Content[0], map[string]any{}); err == nil {
		t.Error("a nested human-only member was accepted, though the agent gate reads top-level members only")
	}
}

func agentValuedBody(t *testing.T, member string) *yaml.Node {
	t.Helper()
	var node yaml.Node
	if err := yaml.Unmarshal([]byte(`
content:
  application/json:
    schema:
      type: object
      properties:
        source: `+member+`
`), &node); err != nil {
		t.Fatal(err)
	}
	return node.Content[0]
}

// An agent is offered only the values it may send, and the gate is told them.
func TestAnAgentIsOfferedOnlyTheValuesItMaySend(t *testing.T) {
	got, marks, err := bodySchema(agentValuedBody(t,
		`{ type: string, enum: [browser, chosen, agent], x-agent-values: [agent], description: Only an agent sends agent. }`), map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"properties":{"source":{"description":"Only an agent sends agent.","enum":["agent"],"type":"string"}},"type":"object"}`
	if got != want {
		t.Errorf("body schema %s, want %s", got, want)
	}
	if !slices.Equal(marks.AgentValues["source"], []string{"agent"}) {
		t.Errorf("agent values %v, want source: [agent] for the agent gate", marks.AgentValues)
	}
	for _, member := range []string{
		`{ type: string, enum: [browser, agent], x-agent-values: [chosen] }`,
		`{ type: string, x-agent-values: [agent] }`,
		`{ type: object, properties: { kind: { type: string, enum: [a, b], x-agent-values: [a] } } }`,
	} {
		if _, _, err := bodySchema(agentValuedBody(t, member), map[string]any{}); err == nil {
			t.Errorf("%s was accepted; a value outside the enum, a member with no enum and a nested marker are refused", member)
		}
	}
}

// The words that state what a caller may send are on the field, in a body only.
func TestABodyFieldKeepsItsDescription(t *testing.T) {
	schemas := map[string]any{"Zone": map[string]any{"type": "string", "description": "An IANA zone name."}}
	got, _, err := bodySchema(agentValuedBody(t, `{ $ref: '#/components/schemas/Zone' }`), schemas)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, `"description":"An IANA zone name."`) {
		t.Errorf("body schema %s, want the referenced schema's description on the field", got)
	}
}
