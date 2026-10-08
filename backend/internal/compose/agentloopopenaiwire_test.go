// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/platform/config"
)

// openAIRootRefusal is OpenAI's answer to a response_format schema whose root
// is not an object, in strict and non-strict mode alike.
const openAIRootRefusal = `{"error":{"type":"invalid_request_error","code":"invalid_json_schema",` +
	`"message":"Invalid schema for response_format 'structured_output': schema must be a JSON Schema of 'type: \"object\"', got 'type: \"None\"'."}}`

// openAIRootRuleEndpoint is a chat-completions host that applies OpenAI's root
// rule, and otherwise answers the final step in whatever object the schema asks
// for: bare when the root declares the step's keys, else under its one property.
func openAIRootRuleEndpoint(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			ResponseFormat struct {
				JSONSchema struct {
					Schema struct {
						Type       any                        `json:"type"`
						Properties map[string]json.RawMessage `json:"properties"`
					} `json:"schema"`
				} `json:"json_schema"`
			} `json:"response_format"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		root := body.ResponseFormat.JSONSchema.Schema
		w.Header().Set("Content-Type", "application/json")
		if root.Type != "object" {
			w.WriteHeader(http.StatusBadRequest)
			writeTestBody(t, w, openAIRootRefusal)
			return
		}
		answer := `{"final":{"summary":"nothing to do"}}`
		if _, bare := root.Properties["final"]; !bare && len(root.Properties) == 1 {
			for name := range root.Properties {
				answer = `{"` + name + `":` + answer + `}`
			}
		}
		content, err := json.Marshal(answer)
		if err != nil {
			t.Errorf("encoding the answer: %v", err)
			return
		}
		writeTestBody(t, w, `{"model":"gpt-test","choices":[{"finish_reason":"stop","message":{"role":"assistant","content":`+
			string(content)+`}}],"usage":{"prompt_tokens":10,"completion_tokens":5}}`)
	}))
}

func writeTestBody(t *testing.T, w http.ResponseWriter, body string) {
	t.Helper()
	if _, err := w.Write([]byte(body)); err != nil {
		t.Errorf("writing the response: %v", err)
	}
}

// The runner's step schema is a union at its root, and the OpenAI wire refuses
// any root but an object: a run on an OpenAI model must still be sent a schema
// that wire takes, and must read back the step the model answered.
func TestAnAgentLoopStepRoundTripsTheOpenAIWire(t *testing.T) {
	srv := openAIRootRuleEndpoint(t)
	defer srv.Close()
	client, err := ai.SelectBrain(ai.ProviderConfig{Provider: "openai_compatible", BaseURL: srv.URL, Model: "gpt-test"},
		config.Static(map[string]string{"OPENAI_COMPATIBLE_API_KEY": "test-key"}))
	if err != nil {
		t.Fatalf("binding the openai_compatible client: %v", err)
	}
	prepared, err := agentLoopCases{agent: agentLoopAgent}.Prepare(
		agentLoopFixtureJSON(t, agentLoopBaseFixture()), agentLoopExpectationJSON(t, agentLoopFinalStep))
	if err != nil {
		t.Fatalf("preparing the case: %v", err)
	}
	trace, err := prepared.Run(context.Background(), client)
	if err != nil {
		t.Fatalf("the step never came back over the OpenAI wire: %v", err)
	}
	if got := prepared.Evaluate(trace); got.Result != aitasks.OutcomeAccepted {
		t.Fatalf("the runner did not read the step the model answered: %s (%s); reply %q", got.Result, got.Detail, trace.Output)
	}
}
