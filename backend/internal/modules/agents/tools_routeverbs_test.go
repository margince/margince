// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

var shareRecordVerb = RouteVerb{
	Verb: "share_record", Router: "change_access", Action: "share", Op: "createRecordGrant",
	Summary: "Share a record", Method: "POST", Pattern: "/v1/record-grants", Scope: principal.ScopeWrite,
	Body: true, AnswerSchema: json.RawMessage(`{"type":"object","properties":{"id":{"type":"string"}}}`),
}

var revertStageVerb = RouteVerb{
	Verb: "revert_stage_move", Router: "change_state", Action: "revert_stage", Op: "revertDealStage",
	Summary: "Revert a stage move", Method: "POST", Pattern: "/v1/deals/{id}/stage-revert",
	Scope: principal.ScopeWrite, PathParams: []string{"id"}, IfMatch: true, AskUserFirst: true,
	Query:        []ViewQuery{{Name: "kinds", Type: schemaArray}},
	AnswerSchema: json.RawMessage(`{"type":"object","properties":{"status":{"type":"integer"}}}`),
}

func TestARouteVerbPassesEachArgumentWhereItsRouteTakesIt(t *testing.T) {
	var got RouteCall
	tool := routeVerbTool{verb: revertStageVerb, run: func(_ context.Context, call RouteCall) (json.RawMessage, error) {
		got = call
		return json.RawMessage(`{}`), nil
	}}
	if _, err := tool.Handle(context.Background(), json.RawMessage(`{"id":"d1","if_version":3,"kinds":["a","b"]}`)); err != nil {
		t.Fatal(err)
	}
	if got.Path["id"] != "d1" || got.Version != "3" || strings.Join(got.Query["kinds"], ",") != "a,b" {
		t.Errorf("call = %+v", got)
	}
	_, err := tool.Handle(context.Background(), json.RawMessage(`{"if_version":3}`))
	var bad *BadArgsError
	if !errors.As(err, &bad) || bad.Field != "id" {
		t.Errorf("a call missing its path id answered %v, want a bad argument naming id", err)
	}
	if _, err := tool.Handle(context.Background(), json.RawMessage(`{"id":"d1","stage":"x"}`)); !errors.As(err, &bad) {
		t.Errorf("an argument the route does not take answered %v, want a bad argument", err)
	}
}

func TestARouteVerbsSchemaIsItsRoutesParameters(t *testing.T) {
	spec := routeVerbTool{verb: revertStageVerb}.Spec()
	var schemaDoc struct {
		Required   []string                   `json:"required"`
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal(spec.InputSchema, &schemaDoc); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"id", "if_version", "kinds"} {
		if schemaDoc.Properties[name] == nil {
			t.Errorf("schema lacks %s: %s", name, spec.InputSchema)
		}
	}
	if !strings.Contains(string(schemaDoc.Properties["kinds"]), `"type":"array"`) {
		t.Errorf("a list parameter is published as %s, which refuses the list its route takes", schemaDoc.Properties["kinds"])
	}
	if schemaDoc.Properties["request_body"] != nil || strings.Join(schemaDoc.Required, ",") != "id" {
		t.Errorf("schema = %s", spec.InputSchema)
	}
	if !spec.Unlisted || spec.Tier != mcp.TierAutoExecute || !spec.AskUserFirst || spec.OpenAPIOp != "revertDealStage" {
		t.Errorf("spec = %+v", spec)
	}
	share := routeVerbTool{verb: shareRecordVerb}.Spec()
	if share.UnkeyedArguments["$.request_body"] == "" {
		t.Errorf("a body-taking verb declares no unkeyed body: %+v", share.UnkeyedArguments)
	}
}

func TestARouteVerbRequiresTheQueryItsRouteRequires(t *testing.T) {
	verb := revertStageVerb
	verb.Query = []ViewQuery{
		{Name: "entity_type", Type: "string", Required: true, Enum: []string{"company", "deal"}},
		{Name: "since", Type: "string", Format: "date-time"},
		{Name: "kinds", Type: schemaArray, Enum: []string{"a"}},
	}
	got := string(routeVerbTool{verb: verb}.Spec().InputSchema)
	for _, want := range []string{
		`"required":["id","entity_type"]`, `"entity_type":{"type":"string","enum":["company","deal"]}`,
		`"since":{"type":"string","format":"date-time"}`, `"kinds":{"type":"array","items":{"type":"string","enum":["a"]}}`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the input schema %s lacks %s", got, want)
		}
	}
}

func TestAContractRouterListsTheContractsActions(t *testing.T) {
	r := NewRegistry(nil, nil)
	RegisterRouteVerbs(r, []RouteVerb{shareRecordVerb}, nil)
	registerContractRouters(r)
	spec, ok := r.Spec("change_access")
	if !ok || len(spec.Actions) != 1 || spec.Actions[0].Action != "share" || spec.Actions[0].Verb != "share_record" {
		t.Fatalf("change_access = %+v, %v", spec.Actions, ok)
	}
	if _, listed := r.Spec("change_offers"); listed {
		t.Error("a contract router the contract gave no action is listed")
	}
	verb, _ := r.Spec("share_record")
	if strings.Contains(string(verb.InputSchema), idempotencyKeyArg) {
		t.Error("a route verb advertises the retry key it cannot honour")
	}
}

func TestAContractActionJoinsAHandWrittenRouter(t *testing.T) {
	r := NewRegistry(nil, nil)
	r.Register(&argsTool{spec: targetSpec("move_it", principal.ScopeWrite, mcp.TierAutoExecute)})
	RegisterRouteVerbs(r, []RouteVerb{revertStageVerb}, nil)
	r.Register(fakeRouter{name: "change_state", targets: map[string]string{"move": "move_it"}})
	spec, _ := r.Spec("change_state")
	actions := map[string]string{}
	for _, a := range spec.Actions {
		actions[a.Action] = a.Verb
	}
	if actions["move"] != "move_it" || actions["revert_stage"] != "revert_stage_move" {
		t.Errorf("actions = %v, want the router's own and the contract's", actions)
	}
	target, _, err := r.resolve("change_state", json.RawMessage(`{"action":"revert_stage","id":"d1"}`))
	if err != nil || target != "revert_stage_move" {
		t.Errorf("resolve = %s, %v", target, err)
	}
}

func TestAContractActionCannotRespellARoutersOwn(t *testing.T) {
	r := NewRegistry(nil, nil)
	r.Register(&argsTool{spec: targetSpec("move_it", principal.ScopeWrite, mcp.TierAutoExecute)})
	clash := revertStageVerb
	clash.Action = "move"
	RegisterRouteVerbs(r, []RouteVerb{clash}, nil)
	defer func() {
		if recover() == nil {
			t.Error("a contract action spelled like the router's own registered")
		}
	}()
	r.Register(fakeRouter{name: "change_state", targets: map[string]string{"move": "move_it"}})
}

func TestReadWorkspaceListsWhatARoutersActionsTake(t *testing.T) {
	withBody := shareRecordVerb
	withBody.BodySchema = json.RawMessage(`{"type":"object","required":["record_id"]}`)
	r := NewRegistry(nil, nil)
	RegisterRouteVerbs(r, []RouteVerb{withBody, revertStageVerb}, nil)
	if got := r.routeActions["read_workspace"]["actions"]; got != "describe_router_actions" {
		t.Errorf("read_workspace topic actions runs %q, want describe_router_actions", got)
	}
	out, err := describeRouterActions{verbs: []RouteVerb{withBody, revertStageVerb}}.Handle(context.Background(),
		json.RawMessage(`{"router":"change_access"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), `"action":"share"`) || !strings.Contains(string(out), `"required":["record_id"]`) ||
		strings.Contains(string(out), "revert_stage") {
		t.Errorf("answer = %s, want change_access's share with its body and nothing of change_state", out)
	}
	if _, err := (describeRouterActions{verbs: []RouteVerb{withBody}}).Handle(context.Background(), json.RawMessage(`{"router":"x"}`)); err == nil {
		t.Error("an unknown router answered, want a bad argument")
	}
}

// A write that overflows answers {status, truncated}, so its schema admits that
// and holds nothing back as required. A read is refused instead, and keeps its own.
func TestAWritesAnswerSchemaAdmitsTheCutAnswer(t *testing.T) {
	answer := json.RawMessage(`{"type":"object","required":["id"],"properties":{"id":{"type":"string"}}}`)
	cut := json.RawMessage(`{"status":201,"truncated":true}`)
	write := RouteVerb{Verb: "w", Method: "POST", Pattern: "/v1/w", Scope: principal.ScopeWrite, AnswerSchema: answer}
	if defect := ResultDefect(routeVerbTool{verb: write}.Spec().OutputSchema, cut); defect != "" {
		t.Errorf("a cut write answer breaks its own schema: %s", defect)
	}
	if defect := ResultDefect(routeVerbTool{verb: write}.Spec().OutputSchema, json.RawMessage(`{"id":"d1"}`)); defect != "" {
		t.Errorf("a whole write answer breaks its own schema: %s", defect)
	}
	write.AnswerSchema = json.RawMessage(`{"type":"object","properties":{"status":{"type":"string"}}}`)
	for _, whole := range []string{`{"status":"draft"}`, string(cut)} {
		if defect := ResultDefect(routeVerbTool{verb: write}.Spec().OutputSchema, json.RawMessage(whole)); defect != "" {
			t.Errorf("a write whose record has a status answered %s against its own schema: %s", whole, defect)
		}
	}
	read := RouteVerb{Verb: "r", Method: "GET", Pattern: "/v1/r", Scope: principal.ScopeRead, AnswerSchema: answer}
	if defect := ResultDefect(routeVerbTool{verb: read}.Spec().OutputSchema, json.RawMessage(`{}`)); defect == "" {
		t.Error("a read answer missing a required member passed its schema")
	}
}
