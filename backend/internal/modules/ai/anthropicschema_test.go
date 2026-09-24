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
	"github.com/margince/margince/backend/internal/shared/schema"
)

// What each schema shape is sent as, compared byte for byte: a fitted schema
// keeps its caller's key order, so the expected strings are written in the
// order the fit produces.
func TestAnthropicOutputSchemaFitsWhatTheDecoderEnforces(t *testing.T) {
	cases := []struct {
		name      string
		schema    string
		downgrade string
		// sent is the schema expected on the wire, compacted; empty means
		// nothing is sent.
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
			sent:      `{"type":"object","additionalProperties":false,"properties":{"subject":{"type":"string","description":"The subject line.\n\n{minLength: 1, maxLength: 998}"}},"required":["subject"]}`,
		},
		{
			name:      "numeric bounds inside array items move too",
			schema:    `{"type":"object","additionalProperties":false,"properties":{"scores":{"type":"array","items":{"type":"number","minimum":0,"maximum":1}}},"required":["scores"]}`,
			downgrade: model.SchemaRelaxed,
			sent:      `{"type":"object","additionalProperties":false,"properties":{"scores":{"type":"array","items":{"type":"number","description":"{minimum: 0, maximum: 1}"}}},"required":["scores"]}`,
		},
		{
			name:      "array bounds and an unlisted format move",
			schema:    `{"type":"array","maxItems":5,"minItems":2,"uniqueItems":true,"items":{"type":"string","format":"phone"}}`,
			downgrade: model.SchemaRelaxed,
			sent:      `{"type":"array","items":{"type":"string","description":"{format: \"phone\"}"},"description":"{maxItems: 5, minItems: 2, uniqueItems: true}"}`,
		},
		{
			// Closing enforces more than was written, not less, so it is sent
			// fitted but reported as no downgrade.
			name:   "an object with properties but no closure is closed",
			schema: `{"type":"object","properties":{"a":{"type":"string"}},"required":["a"]}`,
			sent:   `{"type":"object","properties":{"a":{"type":"string"}},"required":["a"],"additionalProperties":false}`,
		},
		{
			name:      "properties keep the order they were written in",
			schema:    `{"type":"object","additionalProperties":false,"properties":{"subject":{"type":"string","maxLength":9},"body":{"type":"string"},"acknowledged":{"type":"boolean"}},"required":["subject","body","acknowledged"]}`,
			downgrade: model.SchemaRelaxed,
			sent:      `{"type":"object","additionalProperties":false,"properties":{"subject":{"type":"string","description":"{maxLength: 9}"},"body":{"type":"string"},"acknowledged":{"type":"boolean"}},"required":["subject","body","acknowledged"]}`,
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
		{
			name:   "a local reference to the root's definitions goes as written",
			schema: `{"type":"object","additionalProperties":false,"properties":{"home":{"$ref":"#/$defs/address"},"work":{"$ref":"#/definitions/address"}},"required":["home","work"],"$defs":{"address":{"type":"object","additionalProperties":false,"properties":{"city":{"type":"string"}},"required":["city"]}},"definitions":{"address":{"type":"object","additionalProperties":false,"properties":{"city":{"type":"string"}},"required":["city"]}}}`,
			sent:   `{"type":"object","additionalProperties":false,"properties":{"home":{"$ref":"#/$defs/address"},"work":{"$ref":"#/definitions/address"}},"required":["home","work"],"$defs":{"address":{"type":"object","additionalProperties":false,"properties":{"city":{"type":"string"}},"required":["city"]}},"definitions":{"address":{"type":"object","additionalProperties":false,"properties":{"city":{"type":"string"}},"required":["city"]}}}`,
		},
		{
			name:      "a definition is fitted like any subschema",
			schema:    `{"type":"object","properties":{"id":{"$ref":"#/$defs/id"}},"required":["id"],"$defs":{"id":{"type":"string","maxLength":8}}}`,
			downgrade: model.SchemaRelaxed,
			sent:      `{"type":"object","properties":{"id":{"$ref":"#/$defs/id"}},"required":["id"],"$defs":{"id":{"type":"string","description":"{maxLength: 8}"}},"additionalProperties":false}`,
		},
		{name: "an external reference is dropped", schema: `{"$ref":"https://example.com/schema.json"}`, downgrade: model.SchemaDropped},
		{name: "a reference to a missing definition is dropped", schema: `{"$ref":"#/$defs/y","$defs":{"x":{"type":"string"}}}`, downgrade: model.SchemaDropped},
		{name: "a reference to the root is dropped", schema: `{"anyOf":[{"type":"null"},{"$ref":"#"}]}`, downgrade: model.SchemaDropped},
		{name: "a self-referencing definition is dropped", schema: `{"$ref":"#/$defs/node","$defs":{"node":{"type":"object","properties":{"next":{"anyOf":[{"type":"null"},{"$ref":"#/$defs/node"}]}},"required":["next"]}}}`, downgrade: model.SchemaDropped},
		{name: "mutually recursive definitions are dropped", schema: `{"$ref":"#/$defs/a","$defs":{"a":{"type":"array","items":{"$ref":"#/$defs/b"}},"b":{"type":"array","items":{"$ref":"#/$defs/a"}}}}`, downgrade: model.SchemaDropped},
		{name: "a reference inside allOf is dropped", schema: `{"allOf":[{"$ref":"#/$defs/x"}],"$defs":{"x":{"type":"string"}}}`, downgrade: model.SchemaDropped},
		{name: "definitions below the root are dropped", schema: `{"type":"object","properties":{"a":{"type":"string","$defs":{"x":{"type":"string"}}}},"required":["a"]}`, downgrade: model.SchemaDropped},
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
			if !bytes.Equal(compacted(t, sent), compacted(t, json.RawMessage(tc.sent))) {
				t.Fatalf("sent %s\nwant %s", sent, tc.sent)
			}
		})
	}
}

// What the shared builder composes is inside the decoder's subset, so it goes
// byte for byte — every constructor, nested the way task schemas nest them. A
// builder shape that started drawing a downgrade would quietly weaken
// generation for every Anthropic-bound task that uses it.
func TestAnthropicSendsWhatTheSchemaBuilderComposesVerbatim(t *testing.T) {
	leaf := schema.Record(
		schema.Field("text", schema.String().Describe("what it says")),
		schema.Field("score", schema.Number()),
		schema.Field("count", schema.Integer()),
		schema.Field("kind", schema.Enum("a", "b")),
	)
	raw := schema.Must(schema.Record(
		schema.Field("leaf", leaf),
		schema.Field("optional", schema.Optional(leaf)),
		schema.Field("list", schema.Array(schema.Optional(schema.Array(leaf)))),
		schema.Field("object", schema.Object(map[string]schema.Node{"id": schema.String()}, "id")),
	))
	sent, downgrade := anthropicOutputSchema(raw)
	if downgrade != "" || !bytes.Equal(sent, raw) {
		t.Fatalf("a builder schema was fitted (downgrade %q):\nsent %s\nwant %s", downgrade, sent, raw)
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

// compacted is a JSON document without insignificant whitespace, so two
// spellings compare by content and key order alone.
func compacted(t *testing.T, raw json.RawMessage) []byte {
	t.Helper()
	var out bytes.Buffer
	if err := json.Compact(&out, raw); err != nil {
		t.Fatalf("compacting %s: %v", raw, err)
	}
	return out.Bytes()
}
