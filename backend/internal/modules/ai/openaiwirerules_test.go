// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// The Responses and embeddings wires state limits a request must keep and
// outcomes a reply can carry. Each case here is one of them, as the API
// documents it.

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// A phased model sends its working notes as `commentary` messages before the
// `final_answer` one; the notes are not the answer, in either mode.
func TestAnOpenAIReplyIsTheFinalAnswerNotTheCommentary(t *testing.T) {
	const phased = `{"status":"completed","output":[` +
		`{"type":"message","phase":"commentary","content":[{"type":"output_text","text":"checking the record"}]},` +
		`{"type":"message","phase":"final_answer","content":[{"type":"output_text","text":"{\"ok\":true}"}]}]}`
	client := newOpenAIForTest(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(phased))
	})
	msgs := []model.Message{{Role: "user", Content: "x"}}
	for name, req := range map[string]model.Request{
		"schema-bound": {Messages: msgs, ResponseSchema: json.RawMessage(`{"type":"object"}`)},
		"free text":    {Messages: msgs},
	} {
		t.Run(name, func(t *testing.T) {
			resp, err := client.Complete(context.Background(), req)
			if err != nil {
				t.Fatal(err)
			}
			if resp.Text != `{"ok":true}` {
				t.Errorf("reply = %q, want the final answer alone", resp.Text)
			}
		})
	}
}

// Only text-embedding-3 and later take `dimensions`; ada-002 answers any value
// with a 400, so the field is left off for it and kept for the rest.
func TestOpenAIEmbedSendsDimensionsOnlyToAModelThatTakesThem(t *testing.T) {
	for embedModel, wantSent := range map[string]bool{"text-embedding-ada-002": false, "text-embedding-3-small": true} {
		t.Run(embedModel, func(t *testing.T) {
			var body map[string]json.RawMessage
			client := newOpenAIForTest(t, func(w http.ResponseWriter, r *http.Request) {
				if err := json.Unmarshal(readBody(t, r.Body), &body); err != nil {
					t.Errorf("body not JSON: %v", err)
				}
				_, _ = w.Write([]byte(`{"data":[{"embedding":[0.1]}]}`))
			})
			if _, err := client.Embed(context.Background(), model.EmbedRequest{Model: embedModel, Inputs: []string{"a"}, Dimensions: 1536}); err != nil {
				t.Fatal(err)
			}
			if _, sent := body["dimensions"]; sent != wantSent {
				t.Errorf("dimensions sent = %v, want %v (body %v)", sent, wantSent, body)
			}
		})
	}
}

// GPT-6 refuses minimal with a 400 naming low as its shallowest level, so an
// admin's minimal reaches it as low. A family that takes minimal keeps it.
func TestAnOpenAIThinkingLevelIsRaisedToTheFamilysShallowest(t *testing.T) {
	for name, tc := range map[string]struct{ model, level, want string }{
		"gpt-6.1-sol refuses minimal": {"gpt-6.1-sol", "minimal", `{"effort":"low"}`},
		"gpt-5.1 refuses minimal":     {"gpt-5.1", "minimal", `{"effort":"low"}`},
		"gpt-5-mini takes minimal":    {"gpt-5-mini", "minimal", `{"effort":"minimal"}`},
		"gpt-6.1-sol takes high":      {"gpt-6.1-sol", "high", `{"effort":"high"}`},
	} {
		t.Run(name, func(t *testing.T) {
			body := openaiRequestBody(t, model.Request{Model: tc.model, ThinkingLevel: tc.level})
			if got := string(body["reasoning"]); got != tc.want {
				t.Errorf("reasoning = %s, want %s", got, tc.want)
			}
		})
	}
}

// The Responses API refuses a max_output_tokens under 16.
func TestOpenAIRaisesATinyOutputCeilingToTheAPIMinimum(t *testing.T) {
	for asked, want := range map[int]string{1: "16", 15: "16", 16: "16", 300: "300"} {
		body := openaiRequestBody(t, model.Request{MaxTokens: asked})
		if got := string(body["max_output_tokens"]); got != want {
			t.Errorf("asked %d: max_output_tokens = %s, want %s", asked, got, want)
		}
	}
}

// A failed response states its cause as a code. A policy code withholds the
// answer; a rate limit is the throttle a 429 carrying the same words is.
func TestAFailedOpenAIResponseCodeKeepsItsMeaning(t *testing.T) {
	for code, want := range map[string]error{
		"misalignment_policy_violation": model.ErrOutputWithheld,
		"rate_limit_exceeded":           ErrProviderThrottled,
	} {
		t.Run(code, func(t *testing.T) {
			failed := `{"status":"failed","error":{"code":"` + code + `","message":"m"}}`
			client := newOpenAIForTest(t, func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(failed))
			})
			_, err := client.Complete(context.Background(), model.Request{Messages: []model.Message{{Role: "user", Content: "q"}}})
			if !errors.Is(err, want) {
				t.Errorf("a failed response with %s read as %v, want %v", code, err, want)
			}
		})
	}
}

// The stream's own `error` event names the same codes and means the same.
func TestAStreamRateLimitEventIsAThrottle(t *testing.T) {
	client := newOpenAIForTest(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `data: {"type":"error","code":"rate_limit_exceeded","message":"slow down"}`+"\n\n")
	})
	stream, err := client.Stream(context.Background(), model.Request{Messages: []model.Message{{Role: "user", Content: "hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := stream.Close(); err != nil {
			t.Errorf("closing stream: %v", err)
		}
	})
	if _, _, err := stream.Next(context.Background()); !errors.Is(err, ErrProviderThrottled) {
		t.Errorf("a rate-limit stream event read as %v, want a throttle", err)
	}
}

// openaiRequestBody is the Responses request body the native adapter sends for req.
func openaiRequestBody(t *testing.T, req model.Request) map[string]json.RawMessage {
	t.Helper()
	var body map[string]json.RawMessage
	client := newOpenAIForTest(t, func(w http.ResponseWriter, r *http.Request) {
		if err := json.Unmarshal(readBody(t, r.Body), &body); err != nil {
			t.Errorf("body not JSON: %v", err)
		}
		_, _ = w.Write([]byte(`{"status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":"ok"}]}]}`))
	})
	req.Messages = []model.Message{{Role: "user", Content: "hi"}}
	if _, err := client.Complete(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	return body
}
