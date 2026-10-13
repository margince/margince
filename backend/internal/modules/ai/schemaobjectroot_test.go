// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/model"
)

const unionRootSchema = `{"anyOf":[{"type":"object","properties":{"a":{"type":"string"}},"required":["a"],"additionalProperties":false}]}`

// The Responses API holds the same root rule as chat completions. A union
// root goes out under one property, and its answer comes back without it.
func TestTheResponsesWireSendsAUnionRootAsAnObjectAndAnswersTheUnion(t *testing.T) {
	var sent struct {
		Text struct {
			Format struct {
				Schema struct {
					Type       string                     `json:"type"`
					Properties map[string]json.RawMessage `json:"properties"`
				} `json:"schema"`
				Strict bool `json:"strict"`
			} `json:"format"`
		} `json:"text"`
	}
	client := newOpenAIForTest(t, func(w http.ResponseWriter, r *http.Request) {
		if err := json.Unmarshal(readBody(t, r.Body), &sent); err != nil {
			t.Errorf("decoding the request: %v", err)
		}
		writeOrFail(t, w, `{"model":"gpt-x","status":"completed","output":[{"type":"message","content":[`+
			`{"type":"output_text","text":"{\"value\":{\"a\":\"x\"}}"}]}]}`)
	})
	resp, err := client.Complete(context.Background(), model.Request{
		Messages:       []model.Message{{Role: "user", Content: "x"}},
		ResponseSchema: json.RawMessage(unionRootSchema),
	})
	if err != nil {
		t.Fatal(err)
	}
	if format := sent.Text.Format; format.Schema.Type != kwObject || len(format.Schema.Properties) != 1 {
		t.Fatalf("sent a root the wire refuses: %+v", format.Schema)
	}
	if !sent.Text.Format.Strict || resp.SchemaDowngrade != "" {
		t.Errorf("a wrapped schema that is strict-clean went unenforced: strict=%v downgrade=%q",
			sent.Text.Format.Strict, resp.SchemaDowngrade)
	}
	if resp.Text != `{"a":"x"}` {
		t.Errorf("caller got %q, want the union's own answer", resp.Text)
	}
}

// A schema already rooted in an object is sent byte for byte. Its answer is
// never unwrapped, even when its own one key is the wrapper's name.
func TestAnObjectRootGoesOutAsWrittenAndItsAnswerUntouched(t *testing.T) {
	const objectRoot = `{"type":"object","properties":{"value":{"type":"string"}},"required":["value"],"additionalProperties":false}`
	const answer = `{"value":"x"}`
	var sent struct {
		ResponseFormat struct {
			JSONSchema struct {
				Schema json.RawMessage `json:"schema"`
			} `json:"json_schema"`
		} `json:"response_format"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.Unmarshal(readBody(t, r.Body), &sent); err != nil {
			t.Errorf("decoding the request: %v", err)
		}
		content, err := json.Marshal(answer)
		if err != nil {
			t.Errorf("encoding the answer: %v", err)
		}
		writeOrFail(t, w, `{"choices":[{"finish_reason":"stop","message":{"content":`+string(content)+`}}]}`)
	}))
	t.Cleanup(srv.Close)
	c := &openAICompatClient{http: &http.Client{}, baseURL: srv.URL, defaultModel: "m"}
	resp, err := c.Complete(context.Background(), model.Request{ResponseSchema: json.RawMessage(objectRoot)})
	if err != nil {
		t.Fatal(err)
	}
	if string(sent.ResponseFormat.JSONSchema.Schema) != objectRoot {
		t.Errorf("sent %s, want the schema as written", sent.ResponseFormat.JSONSchema.Schema)
	}
	if resp.Text != answer {
		t.Errorf("caller got %q, want %q", resp.Text, answer)
	}
}

// Only the exact wrapper is unwrapped: a reply cut off mid-document, answered
// bare, or carrying a second key is the caller's parser's to refuse.
func TestOnlyTheExactWrapperIsUnwrapped(t *testing.T) {
	for name, reply := range map[string]string{
		"cut off":    `{"value":{"a":"x`,
		"bare":       `{"a":"x"}`,
		"second key": `{"value":{"a":"x"},"b":1}`,
		"not json":   `I cannot answer that.`,
	} {
		if got := unwrapObjectRoot(reply); got != reply {
			t.Errorf("%s: %q became %q", name, reply, got)
		}
	}
}

// A model that fences its JSON has answered correctly, wrapper and all.
func TestAFencedWrapperIsUnwrapped(t *testing.T) {
	if got := unwrapObjectRoot("```json\n{\"value\":{\"a\":\"x\"}}\n```"); got != `{"a":"x"}` {
		t.Errorf("caller got %q, want the union's own answer", got)
	}
}

func writeOrFail(t *testing.T, w http.ResponseWriter, body string) {
	t.Helper()
	if _, err := w.Write([]byte(body)); err != nil {
		t.Errorf("writing the response: %v", err)
	}
}
