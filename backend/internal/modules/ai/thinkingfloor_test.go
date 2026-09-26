// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// openRouterModelList is the broker's /api/v1/models as it described this
// tree's models on 2026-09-26, trimmed to the fields the floor reads.
const openRouterModelList = `{"data":[
	{"id":"openai/gpt-oss-120b","reasoning":{"mandatory":true,"supported_efforts":["high","medium","low"],"default_effort":"medium"}},
	{"id":"mistralai/mistral-medium-3-5","reasoning":{"supported_efforts":["high","none"],"default_effort":"high"}},
	{"id":"mistralai/mistral-small-2603","reasoning":{"default_enabled":false,"supported_efforts":["high","none"],"default_effort":"high"}},
	{"id":"google/gemma-4-31b-it","reasoning":{"default_enabled":false}},
	{"id":"anthropic/claude-sonnet-4.6","reasoning":{"supported_efforts":["max","high","medium","low"],"default_effort":"medium"}},
	{"id":"mistralai/ministral-8b-2512","reasoning":null}
]}`

// brokerStub stands in for OpenRouter: it serves modelList (or modelsStatus)
// on the model list and records every chat body.
type brokerStub struct {
	modelsStatus int
	modelsCalls  int
	chats        []map[string]json.RawMessage
}

// redirectTo sends every request to srv, so a binding can name the real broker
// host — the only host its floor is mapped for — and still reach the stub.
type redirectTo struct{ target *url.URL }

func (r redirectTo) RoundTrip(req *http.Request) (*http.Response, error) {
	req.URL.Scheme, req.URL.Host = r.target.Scheme, r.target.Host
	return http.DefaultTransport.RoundTrip(req)
}

func newBrokerClient(t *testing.T, stub *brokerStub, modelID string, routing *OpenRouterRouting) model.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v1/models" {
			stub.modelsCalls++
			if stub.modelsStatus != 0 {
				w.WriteHeader(stub.modelsStatus)
				return
			}
			_, _ = w.Write([]byte(openRouterModelList))
			return
		}
		var body map[string]json.RawMessage
		if err := json.Unmarshal(readBody(t, r.Body), &body); err != nil {
			t.Errorf("chat body not JSON: %v", err)
		}
		stub.chats = append(stub.chats, body)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}]}`))
	}))
	t.Cleanup(srv.Close)
	target, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	cfg := ProviderConfig{Provider: providerOpenAICompatible, Model: modelID, BaseURL: "https://openrouter.ai/api", Routing: routing}
	client, err := selectBrainOn(cfg, cloudKeyFor(providerOpenAICompatible, "k"), &http.Client{Transport: redirectTo{target}})
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func askWithFloor(t *testing.T, client model.Client, floor string) {
	t.Helper()
	if _, err := client.Complete(context.Background(), model.Request{
		Messages: []model.Message{{Role: "user", Content: "hi"}}, ThinkingFloor: floor,
	}); err != nil {
		t.Fatal(err)
	}
}

// The broker's floor raises only a model that thinks less than asked, and says
// nothing to one whose own default already meets it or that does not reason.
func TestABrokerFloorRaisesOnlyAModelThatThinksLess(t *testing.T) {
	for name, tc := range map[string]struct {
		model, floor, want string // want "" means no reasoning block
	}{
		"gpt-oss reasons at medium by default":      {"openai/gpt-oss-120b", "low", ""},
		"mistral-medium reasons at high by default": {"mistralai/mistral-medium-3-5", "low", ""},
		"claude reasons at medium by default":       {"anthropic/claude-sonnet-4.6", "low", ""},
		"a floor above the default is raised to":    {"anthropic/claude-sonnet-4.6", "high", `{"effort":"high"}`},
		"mistral-small is off; high is its lowest":  {"mistralai/mistral-small-2603", "low", `{"effort":"high"}`},
		"gemma is off and grades no effort":         {"google/gemma-4-31b-it", "low", `{"enabled":true}`},
		"ministral does not reason":                 {"mistralai/ministral-8b-2512", "low", ""},
		"an unlisted model is sent nothing":         {"vendor/unlisted", "low", ""},
		"a request with no floor is sent nothing":   {"google/gemma-4-31b-it", "", ""},
	} {
		t.Run(name, func(t *testing.T) {
			stub := &brokerStub{}
			askWithFloor(t, newBrokerClient(t, stub, tc.model, nil), tc.floor)
			got, sent := stub.chats[0]["reasoning"]
			if tc.want == "" && sent {
				t.Fatalf("reasoning = %s, want none", got)
			}
			if tc.want != "" && string(got) != tc.want {
				t.Fatalf("reasoning = %s, want %s", got, tc.want)
			}
		})
	}
}

// The model list is read once per model, not once per call.
func TestABrokerModelListIsReadOncePerModel(t *testing.T) {
	stub := &brokerStub{}
	client := newBrokerClient(t, stub, "google/gemma-4-31b-it", nil)
	for range 3 {
		askWithFloor(t, client, "low")
	}
	if stub.modelsCalls != 1 {
		t.Fatalf("model list read %d times over three calls, want 1", stub.modelsCalls)
	}
}

// A model list the broker will not serve costs the floor and never the call.
func TestABrokerModelListFailureSendsNoFloorAndServesTheCall(t *testing.T) {
	stub := &brokerStub{modelsStatus: http.StatusInternalServerError}
	askWithFloor(t, newBrokerClient(t, stub, "google/gemma-4-31b-it", nil), "low")
	if got, sent := stub.chats[0]["reasoning"]; sent {
		t.Fatalf("reasoning = %s after a failed model list, want none", got)
	}
}

// A binding's own reasoning_effort is the operator's choice and outranks the
// site floor, in either direction; the model list is then never read.
func TestABindingReasoningEffortOutranksTheFloor(t *testing.T) {
	stub := &brokerStub{}
	client := newBrokerClient(t, stub, "google/gemma-4-31b-it", &OpenRouterRouting{ReasoningEffort: "none"})
	askWithFloor(t, client, "low")
	if got := string(stub.chats[0]["reasoning"]); got != `{"effort":"none"}` {
		t.Fatalf("reasoning = %s, want the binding's none", got)
	}
	if stub.modelsCalls != 0 {
		t.Fatalf("model list read %d times for a binding that names its own effort", stub.modelsCalls)
	}
}

// Anthropic: 4.6-4.8 are turned on adaptively, 4.5 and earlier with the
// minimum budget, and a model that thinks by default or is unknown gets none.
func TestAnAnthropicFloorTurnsOnOnlyAModelThatIsOff(t *testing.T) {
	tool := []model.ToolDef{{Name: "lookup", InputSchema: json.RawMessage(`{"type":"object"}`)}}
	for name, tc := range map[string]struct {
		model     string
		maxTokens int
		tools     []model.ToolDef
		want      string
	}{
		"sonnet 4.6 is off by default":          {"claude-sonnet-4-6", 8192, nil, `{"type":"adaptive"}`},
		"haiku 4.5 takes only a budget":         {"claude-haiku-4-5-20251001", 8192, nil, `{"type":"enabled","budget_tokens":1024}`},
		"a budget that leaves no answer is not": {"claude-haiku-4-5", 1024, nil, ""},
		"opus 5.5 already thinks":               {"claude-opus-5-5", 8192, nil, ""},
		"an unknown model is sent nothing":      {"claude-test", 8192, nil, ""},
		"a tool-carrying request is sent none":  {"claude-sonnet-4-6", 8192, tool, ""},
	} {
		t.Run(name, func(t *testing.T) {
			var body map[string]json.RawMessage
			client := newAnthropicForTest(t, func(w http.ResponseWriter, r *http.Request) {
				if err := json.Unmarshal(readBody(t, r.Body), &body); err != nil {
					t.Errorf("body not JSON: %v", err)
				}
				_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn"}`))
			})
			if _, err := client.Complete(context.Background(), model.Request{
				Model: tc.model, MaxTokens: tc.maxTokens, Tools: tc.tools, ThinkingFloor: "low",
				Messages: []model.Message{{Role: "user", Content: "hi"}},
			}); err != nil {
				t.Fatal(err)
			}
			got, sent := body["thinking"]
			if tc.want == "" && sent {
				t.Fatalf("thinking = %s, want none", got)
			}
			if tc.want != "" && string(got) != tc.want {
				t.Fatalf("thinking = %s, want %s", got, tc.want)
			}
			if _, effort := body["output_config"]; effort {
				t.Fatalf("output_config sent for a floor: %s", body["output_config"])
			}
		})
	}
}

// OpenAI: a none-default family is raised to the floor, a medium-default one
// only by a floor above medium, and a non-reasoning model is never named one.
func TestAnOpenAIFloorRaisesOnlyAShallowerDefault(t *testing.T) {
	low := map[string]json.RawMessage{"openai": json.RawMessage(`{"reasoning_effort":"low"}`)}
	for name, tc := range map[string]struct {
		model, floor string
		opts         map[string]json.RawMessage
		want         string
	}{
		"gpt-5.4 defaults to none":           {"gpt-5.4-mini", "low", nil, `{"effort":"low"}`},
		"minimal asks a none family for low": {"gpt-5.1", "minimal", nil, `{"effort":"low"}`},
		"gpt-5-mini defaults to medium":      {"gpt-5-mini", "low", nil, ""},
		"a floor above medium is raised to":  {"gpt-5-mini", "high", nil, `{"effort":"high"}`},
		"gpt-4.1 does not reason":            {"gpt-4.1", "low", nil, ""},
		"the chat alias does not reason":     {"gpt-5-chat-latest", "high", nil, ""},
		"the request's own effort outranks":  {"gpt-5.4", "high", low, `{"effort":"low"}`},
	} {
		t.Run(name, func(t *testing.T) {
			var body map[string]json.RawMessage
			client := newOpenAIForTest(t, func(w http.ResponseWriter, r *http.Request) {
				if err := json.Unmarshal(readBody(t, r.Body), &body); err != nil {
					t.Errorf("body not JSON: %v", err)
				}
				_, _ = w.Write([]byte(`{"status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":"ok"}]}]}`))
			})
			if _, err := client.Complete(context.Background(), model.Request{
				Model: tc.model, ThinkingFloor: tc.floor, ProviderOptions: tc.opts,
				Messages: []model.Message{{Role: "user", Content: "hi"}},
			}); err != nil {
				t.Fatal(err)
			}
			got, sent := body["reasoning"]
			if tc.want == "" && sent {
				t.Fatalf("reasoning = %s, want none", got)
			}
			if tc.want != "" && string(got) != tc.want {
				t.Fatalf("reasoning = %s, want %s", got, tc.want)
			}
		})
	}
}

// Ollama: a floor replaces the cheapest value with the least thinking that
// meets it; a model that does not think is still sent nothing.
func TestAnOllamaFloorSendsTheLeastThinkingThatMeetsIt(t *testing.T) {
	for name, tc := range map[string]struct {
		show, floor, want string
	}{
		"a boolean model is turned on (gemma4)": {`{"thinking":{"values":[false,true]}}`, "low", "true"},
		"a graded model gets its lowest (oss)":  {`{"thinking":{"values":["high","medium","low"]}}`, "low", `"low"`},
		"a high floor gets high":                {`{"thinking":{"values":["low","medium","high"]}}`, "high", `"high"`},
		"a model that does not think":           {`{"capabilities":["completion"]}`, "low", ""},
		"no floor keeps the cheapest":           {`{"thinking":{"values":[false,true]}}`, "", "false"},
	} {
		t.Run(name, func(t *testing.T) {
			ts := newThinkServer(t, tc.show)
			if _, err := ts.client.Complete(context.Background(), model.Request{
				Messages: []model.Message{{Role: "user", Content: "hi"}}, ThinkingFloor: tc.floor,
			}); err != nil {
				t.Fatal(err)
			}
			got, present := ts.chatWire["think"]
			if tc.want == "" && present {
				t.Fatalf("think = %s, want none", got)
			}
			if tc.want != "" && string(got) != tc.want {
				t.Fatalf("think = %s, want %s", got, tc.want)
			}
		})
	}
}

// vLLM cannot say whether its model thinks, so a floor never reaches its wire.
func TestAVLLMBindingIsSentNoFloor(t *testing.T) {
	var body map[string]json.RawMessage
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.Unmarshal(readBody(t, r.Body), &body); err != nil {
			t.Errorf("body not JSON: %v", err)
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}]}`))
	}))
	t.Cleanup(srv.Close)
	client, err := selectLocalBrain(ProviderConfig{Provider: providerVLLM, Model: "Qwen/Qwen3-14B", BaseURL: srv.URL}, noCloudKeys())
	if err != nil {
		t.Fatal(err)
	}
	askWithFloor(t, client, "low")
	for _, field := range []string{"reasoning", "reasoning_effort", "chat_template_kwargs"} {
		if got, sent := body[field]; sent {
			t.Errorf("%s = %s sent to vLLM", field, got)
		}
	}
}
