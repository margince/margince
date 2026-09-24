// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// What each schema shape is sent as. The shapes are the ones this tree's
// response schemas actually use: a reply draft's string lengths, a score's
// numeric bounds, a capped and de-duplicated list, and a free-form map.
func TestAnthropicOutputSchemaFitsWhatTheDecoderEnforces(t *testing.T) {
	cases := []struct {
		name      string
		schema    string
		downgrade string
		// sent is the schema expected on the wire, compared as JSON; empty
		// means nothing is sent.
		sent string
	}{
		{
			name:   "already eligible goes verbatim",
			schema: `{"type":"object","additionalProperties":false,"properties":{"ok":{"type":"boolean"},"at":{"type":"string","format":"date-time"},"tags":{"type":"array","minItems":1,"items":{"type":"string","enum":["a","b"]}}},"required":["ok"]}`,
			sent:   `{"type":"object","additionalProperties":false,"properties":{"ok":{"type":"boolean"},"at":{"type":"string","format":"date-time"},"tags":{"type":"array","minItems":1,"items":{"type":"string","enum":["a","b"]}}},"required":["ok"]}`,
		},
		{
			name:      "string lengths move into the description",
			schema:    `{"type":"object","additionalProperties":false,"properties":{"subject":{"type":"string","minLength":1,"maxLength":998,"description":"The subject line."}},"required":["subject"]}`,
			downgrade: model.SchemaRelaxed,
			sent:      `{"type":"object","additionalProperties":false,"properties":{"subject":{"type":"string","description":"The subject line.\n\n{maxLength: 998, minLength: 1}"}},"required":["subject"]}`,
		},
		{
			name:      "numeric bounds inside array items move too",
			schema:    `{"type":"object","additionalProperties":false,"properties":{"scores":{"type":"array","items":{"type":"number","minimum":0,"maximum":1}}},"required":["scores"]}`,
			downgrade: model.SchemaRelaxed,
			sent:      `{"type":"object","additionalProperties":false,"properties":{"scores":{"type":"array","items":{"type":"number","description":"{maximum: 1, minimum: 0}"}}},"required":["scores"]}`,
		},
		{
			name:      "array bounds and an unlisted format move",
			schema:    `{"type":"array","maxItems":5,"minItems":2,"uniqueItems":true,"items":{"type":"string","format":"phone"}}`,
			downgrade: model.SchemaRelaxed,
			sent:      `{"type":"array","description":"{maxItems: 5, minItems: 2, uniqueItems: true}","items":{"type":"string","description":"{format: \"phone\"}"}}`,
		},
		{
			name:      "an object with properties but no closure is closed",
			schema:    `{"type":"object","properties":{"a":{"type":"string"}},"required":["a"]}`,
			downgrade: model.SchemaRelaxed,
			sent:      `{"type":"object","additionalProperties":false,"properties":{"a":{"type":"string"}},"required":["a"]}`,
		},
		{
			name:      "each branch of a union is fitted",
			schema:    `{"anyOf":[{"type":"string","maxLength":5},{"type":"null"}]}`,
			downgrade: model.SchemaRelaxed,
			sent:      `{"anyOf":[{"type":"string","description":"{maxLength: 5}"},{"type":"null"}]}`,
		},
		{name: "a free-form object is dropped", schema: `{"type":"object","properties":{"fields":{"type":"object"}}}`, downgrade: model.SchemaDropped},
		{name: "an object open to extra keys is dropped", schema: `{"type":"object","additionalProperties":true,"properties":{"a":{"type":"string"}}}`, downgrade: model.SchemaDropped},
		{name: "a type union is dropped", schema: `{"type":["string","null"]}`, downgrade: model.SchemaDropped},
		{name: "a reference is dropped", schema: `{"$ref":"#/$defs/x","$defs":{"x":{"type":"string"}}}`, downgrade: model.SchemaDropped},
		{name: "an enum of objects is dropped", schema: `{"type":"string","enum":[{"a":1}]}`, downgrade: model.SchemaDropped},
		{name: "tuple items are dropped", schema: `{"type":"array","items":[{"type":"string"}]}`, downgrade: model.SchemaDropped},
		{name: "an unknown keyword is dropped", schema: `{"type":"string","contentEncoding":"base64"}`, downgrade: model.SchemaDropped},
		{name: "a branch that cannot fit drops the whole union", schema: `{"anyOf":[{"type":"string"},{"type":"object"}]}`, downgrade: model.SchemaDropped},
		{name: "unparseable is dropped", schema: `{"type":`, downgrade: model.SchemaDropped},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sent, downgrade := anthropicOutputSchema(json.RawMessage(tc.schema))
			if downgrade != tc.downgrade {
				t.Fatalf("downgrade = %q, want %q", downgrade, tc.downgrade)
			}
			if tc.sent == "" {
				if sent != nil {
					t.Fatalf("a dropped schema must send nothing, sent %s", sent)
				}
				return
			}
			if !jsonEqual(t, sent, json.RawMessage(tc.sent)) {
				t.Fatalf("sent %s\nwant %s", sent, tc.sent)
			}
		})
	}
}

func TestAnthropicOutputSchemaWithNoSchemaSendsNothing(t *testing.T) {
	if sent, downgrade := anthropicOutputSchema(nil); sent != nil || downgrade != "" {
		t.Fatalf("no schema must send nothing and report no downgrade, got %s / %q", sent, downgrade)
	}
}

// The adapter reports the downgrade on the Response, which is what reaches the
// call record, and the wire carries the fitted schema or none.
func TestAnthropicCompleteReportsTheSchemaDowngradeItSentUnder(t *testing.T) {
	cases := []struct {
		name       string
		schema     string
		downgrade  string
		wantFormat bool
	}{
		{"eligible", `{"type":"object","additionalProperties":false,"properties":{"ok":{"type":"boolean"}},"required":["ok"]}`, "", true},
		{"relaxed", `{"type":"object","additionalProperties":false,"properties":{"body":{"type":"string","maxLength":10}},"required":["body"]}`, model.SchemaRelaxed, true},
		{"dropped", `{"type":"object","properties":{"fields":{"type":"object"}}}`, model.SchemaDropped, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var wire map[string]json.RawMessage
			client := newAnthropicForTest(t, func(w http.ResponseWriter, r *http.Request) {
				if err := json.Unmarshal(readBody(t, r.Body), &wire); err != nil {
					t.Errorf("wire not JSON: %v", err)
				}
				writeAnthropicText(t, w, "{}")
			})
			resp, err := client.Complete(context.Background(), model.Request{
				Messages:       []model.Message{{Role: "user", Content: "hi"}},
				ResponseSchema: json.RawMessage(tc.schema),
			})
			if err != nil {
				t.Fatal(err)
			}
			if resp.SchemaDowngrade != tc.downgrade {
				t.Fatalf("SchemaDowngrade = %q, want %q", resp.SchemaDowngrade, tc.downgrade)
			}
			if _, sent := wire["output_config"]; sent != tc.wantFormat {
				t.Fatalf("output_config sent = %v, want %v: %v", sent, tc.wantFormat, wire)
			}
		})
	}
}

// Thinking tokens are itemized under usage.output_tokens_details and already
// counted in output_tokens, so they become ReasoningTokens as reported — on the
// plain wire and on the streamed one, where only the final message_delta
// carries them.
func TestAnthropicReportsThinkingTokensAsReasoningTokens(t *testing.T) {
	t.Run("plain", func(t *testing.T) {
		client := newAnthropicForTest(t, func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.WriteString(w, `{"content":[{"type":"text","text":"ok"}],"usage":{"input_tokens":5,"output_tokens":300,"output_tokens_details":{"thinking_tokens":240}}}`)
		})
		resp, err := client.Complete(context.Background(), model.Request{Messages: []model.Message{{Role: "user", Content: "hi"}}})
		if err != nil {
			t.Fatal(err)
		}
		if resp.OutputTokens != 300 || resp.ReasoningTokens != 240 {
			t.Fatalf("output/reasoning = %d/%d, want 300/240", resp.OutputTokens, resp.ReasoningTokens)
		}
	})
	t.Run("streamed", func(t *testing.T) {
		client := newAnthropicForTest(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, strings.Join([]string{
				`data: {"type":"message_start","message":{"model":"claude-test","usage":{"input_tokens":5}}}`,
				`data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"ok"}}`,
				`data: {"type":"message_delta","usage":{"output_tokens":120}}`,
				`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":300,"output_tokens_details":{"thinking_tokens":240}}}`,
				`data: {"type":"message_stop"}`,
				``,
			}, "\n"))
		})
		resp, err := client.Complete(context.Background(), model.Request{
			MaxTokens: streamedCompleteThreshold + 1,
			Messages:  []model.Message{{Role: "user", Content: "hi"}},
		})
		if err != nil {
			t.Fatal(err)
		}
		if resp.OutputTokens != 300 || resp.ReasoningTokens != 240 {
			t.Fatalf("output/reasoning = %d/%d, want 300/240", resp.OutputTokens, resp.ReasoningTokens)
		}
	})
	t.Run("a count above the output is bounded by it", func(t *testing.T) {
		client := newAnthropicForTest(t, func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.WriteString(w, `{"content":[{"type":"text","text":"ok"}],"usage":{"input_tokens":5,"output_tokens":100,"output_tokens_details":{"thinking_tokens":130}}}`)
		})
		resp, err := client.Complete(context.Background(), model.Request{Messages: []model.Message{{Role: "user", Content: "hi"}}})
		if err != nil {
			t.Fatal(err)
		}
		if resp.ReasoningTokens != 100 {
			t.Fatalf("ReasoningTokens = %d, want it bounded to OutputTokens 100", resp.ReasoningTokens)
		}
	})
}

func writeAnthropicText(t *testing.T, w http.ResponseWriter, text string) {
	t.Helper()
	if err := json.NewEncoder(w).Encode(map[string]any{
		"content": []map[string]any{{"type": "text", "text": text}},
		"usage":   map[string]int{"input_tokens": 1, "output_tokens": 1},
	}); err != nil {
		t.Errorf("encoding fixture response: %v", err)
	}
}

// jsonEqual compares two JSON documents by value, so key order in a fitted
// schema is not mistaken for a difference.
func jsonEqual(t *testing.T, a, b json.RawMessage) bool {
	t.Helper()
	var left, right bytes.Buffer
	if err := json.Compact(&left, canonical(t, a)); err != nil {
		t.Fatalf("compacting %s: %v", a, err)
	}
	if err := json.Compact(&right, canonical(t, b)); err != nil {
		t.Fatalf("compacting %s: %v", b, err)
	}
	return bytes.Equal(left.Bytes(), right.Bytes())
}

// canonical re-encodes a JSON document so every object's keys are sorted, at
// any depth, including inside arrays.
func canonical(t *testing.T, raw json.RawMessage) []byte {
	t.Helper()
	var tree map[string]json.RawMessage
	if err := json.Unmarshal(raw, &tree); err == nil {
		sorted := make(map[string]json.RawMessage, len(tree))
		for key, value := range tree {
			sorted[key] = canonical(t, value)
		}
		return mustMarshal(t, sorted)
	}
	var list []json.RawMessage
	if err := json.Unmarshal(raw, &list); err == nil {
		for i, member := range list {
			list[i] = canonical(t, member)
		}
		return mustMarshal(t, list)
	}
	var scalar json.RawMessage
	if err := json.Unmarshal(raw, &scalar); err != nil {
		t.Fatalf("decoding %s: %v", raw, err)
	}
	return scalar
}

func mustMarshal[T any](t *testing.T, value T) []byte {
	t.Helper()
	out, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("re-encoding: %v", err)
	}
	return out
}
