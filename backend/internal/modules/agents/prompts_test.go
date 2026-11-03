// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// promptDispatcher serves the read tools the read-only prompts name, and none
// of the writing ones.
func promptDispatcher(t *testing.T) *Dispatcher {
	t.Helper()
	registry := NewRegistry(nil, auth.NewGate(fullSeatAuthority{}))
	for _, name := range []string{"read_attention", "read_context", "read_forecast"} {
		registry.Register(echoTool{spec: objectSpec(name, principal.ScopeRead), out: json.RawMessage(`{}`)})
	}
	return NewDispatcher(registry, bindAuthenticated, "margince-crm", "test").WithLogger(discardLog())
}

func callPrompt(ctx context.Context, s *Dispatcher, method, params string) rpcResponse {
	return s.handle(ctx, rpcRequest{
		JSONRPC: jsonRPCVersion, ID: json.RawMessage(`1`), Method: method, Params: json.RawMessage(params),
	}, legacyFraming)
}

func TestAPromptIsOfferedOnlyWhenEveryToolItNamesIs(t *testing.T) {
	resp := callPrompt(scopedAgentCtx(principal.ScopeRead), promptDispatcher(t), methodPromptsList, `{}`)
	if resp.Error != nil {
		t.Fatalf("prompts/list → %q", resp.Error.Message)
	}
	result, ok := resp.Result.(map[string]any)
	if !ok {
		t.Fatalf("prompts/list answered %T", resp.Result)
	}
	listed, ok := result["prompts"].([]map[string]any)
	if !ok {
		t.Fatalf("prompts/list prompts member is %T", result["prompts"])
	}
	names := map[string]bool{}
	for _, entry := range listed {
		name, isText := entry[fieldName].(string)
		if !isText {
			t.Fatalf("a listed prompt has no name: %v", entry)
		}
		names[name] = true
	}
	for _, want := range []string{"whats_slipping", "catch_up", "prepare_meeting", "explain_forecast", "open_promises"} {
		if !names[want] {
			t.Errorf("%s is not listed, though every tool it names is served: %v", want, names)
		}
	}
	for _, absent := range []string{"log_call", "morning_brief"} {
		if names[absent] {
			t.Errorf("%s is listed, and it names a tool this caller is not offered", absent)
		}
	}
}

func TestAPromptExpandsWithTheCallersArguments(t *testing.T) {
	resp := callPrompt(scopedAgentCtx(principal.ScopeRead), promptDispatcher(t), methodPromptsGet,
		`{"name":"catch_up","arguments":{"record":"Acme Robotics"}}`)
	if resp.Error != nil {
		t.Fatalf("prompts/get → %q", resp.Error.Message)
	}
	body, err := json.Marshal(resp.Result)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Messages []struct {
			Role    string `json:"role"`
			Content struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Messages) != 1 || got.Messages[0].Role != "user" || got.Messages[0].Content.Type != fieldText {
		t.Fatalf("prompts/get answered %s, want one user text message", body)
	}
	text := got.Messages[0].Content.Text
	if !strings.Contains(text, "Acme Robotics") || strings.Contains(text, "{record}") {
		t.Errorf("the argument was not filled in: %q", text)
	}
}

func TestAPromptRefusesAMissingArgumentAndAnUnknownName(t *testing.T) {
	s := promptDispatcher(t)
	ctx := scopedAgentCtx(principal.ScopeRead)
	for params, want := range map[string]string{
		`{"name":"catch_up","arguments":{}}`:                         "record",
		`{"name":"no_such_prompt"}`:                                  "prompts/list",
		`{"name":"log_call","arguments":{"with":"Ana","notes":"x"}}`: "no prompt is named",
	} {
		resp := callPrompt(ctx, s, methodPromptsGet, params)
		if resp.Error == nil || resp.Error.Code != codeInvalidParams {
			t.Errorf("%s → %+v, want invalid params", params, resp)
			continue
		}
		if !strings.Contains(resp.Error.Message, want) {
			t.Errorf("%s → %q, want it to say %q", params, resp.Error.Message, want)
		}
	}
}

func TestTheServerAdvertisesPrompts(t *testing.T) {
	capabilities := promptDispatcher(t).capabilities(false)
	if _, ok := capabilities["prompts"]; !ok {
		t.Errorf("capabilities = %v, and a host shows no prompt menu for a server that does not claim one", capabilities)
	}
}
