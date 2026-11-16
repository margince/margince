// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// sentThinkingConfig is the part of a sent request body this file checks.
type sentThinkingConfig struct {
	OutputConfig *anthropicOutputConfig `json:"output_config"`
	Thinking     *anthropicThinking     `json:"thinking"`
}

// sentAnthropicWire sends req through the real client and returns what the
// vendor received of its thinking and output config.
func sentAnthropicWire(t *testing.T, req model.Request) sentThinkingConfig {
	t.Helper()
	var received []byte
	client := newAnthropicForTest(t, func(w http.ResponseWriter, r *http.Request) {
		received = readBody(t, r.Body)
		if err := json.NewEncoder(w).Encode(map[string]any{
			"content": []map[string]any{{"type": "text", "text": "{}"}},
			"usage":   map[string]int{"input_tokens": 1, "output_tokens": 1},
		}); err != nil {
			t.Errorf("encoding fixture response: %v", err)
		}
	})
	if _, err := client.Complete(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	var wire sentThinkingConfig
	if err := json.Unmarshal(received, &wire); err != nil {
		t.Fatalf("wire not JSON: %v", err)
	}
	return wire
}

func TestAnAdminsThinkingLevelReachesAClaude5ModelAsEffort(t *testing.T) {
	cases := []struct {
		name, model, level, floor, wantEffort string
	}{
		{"an exact level is sent as effort", "claude-opus-5-5", effortHigh, "", effortHigh},
		{"minimal asks for the lowest effort the wire has", "claude-sonnet-5-5", effortMinimal, "", effortLow},
		{"a floor never lowers the model's own default", "claude-opus-5-5", "", effortLow, ""},
		{"a model that does not think by default takes no effort", "claude-opus-4-8", effortHigh, "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			wire := sentAnthropicWire(t, model.Request{
				Model:         tc.model,
				Messages:      []model.Message{{Role: "user", Content: "hi"}},
				ThinkingLevel: tc.level,
				ThinkingFloor: tc.floor,
			})
			got := ""
			if wire.OutputConfig != nil {
				got = wire.OutputConfig.Effort
			}
			if got != tc.wantEffort {
				t.Fatalf("effort = %q, want %q", got, tc.wantEffort)
			}
			if tc.wantEffort == "" && wire.OutputConfig != nil {
				t.Fatalf("output_config sent with nothing in it: %+v", wire.OutputConfig)
			}
			if anthropicThinkingModeOf(tc.model) == anthropicThinksByDefault && wire.Thinking != nil {
				t.Fatalf("a thinking block went to a model that rejects one: %+v", wire.Thinking)
			}
		})
	}
}

func TestEffortAndResponseSchemaShareOneOutputConfig(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"ok":{"type":"boolean"}},"required":["ok"]}`)
	wire := sentAnthropicWire(t, model.Request{
		Model:          "claude-opus-5-5",
		Messages:       []model.Message{{Role: "user", Content: "hi"}},
		ThinkingLevel:  effortMedium,
		ResponseSchema: schema,
	})
	if wire.OutputConfig == nil || wire.OutputConfig.Effort != effortMedium || wire.OutputConfig.Format == nil {
		t.Fatalf("output_config must carry both effort and format, got %+v", wire.OutputConfig)
	}
}
