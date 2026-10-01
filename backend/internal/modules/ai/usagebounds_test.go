// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/model"
)

func TestCacheWithinKeepsThePartsInsideThePrompt(t *testing.T) {
	cases := []struct {
		name                string
		prompt, read, write int
		wantRead, wantWrite int
	}{
		{"reported parts fit", 12610, 12601, 0, 12601, 0},
		{"a read and a write together", 1000, 600, 300, 600, 300},
		{"a read above the prompt is the prompt", 100, 130, 0, 100, 0},
		{"a write is bounded by what the read left", 100, 80, 50, 80, 20},
		{"negative reports are none", 100, -5, -1, 0, 0},
		{"no prompt, no cache", 0, 10, 10, 0, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			read, write := cacheWithin(tc.prompt, tc.read, tc.write)
			if read != tc.wantRead || write != tc.wantWrite {
				t.Fatalf("cacheWithin(%d, %d, %d) = %d/%d, want %d/%d",
					tc.prompt, tc.read, tc.write, read, write, tc.wantRead, tc.wantWrite)
			}
		})
	}
}

// Each wire's cache report reaches the port's two buckets. The fixtures are the
// shapes the vendors document: OpenAI's Responses input_tokens_details, a
// broker's prompt_tokens_details (the numbers OpenRouter returned for a cached
// Anthropic prompt), and Ollama's prompt_eval_cached_count.
func TestEveryCacheReportingWireFillsBothCacheBuckets(t *testing.T) {
	cases := []struct {
		name      string
		client    func(t *testing.T, handler http.HandlerFunc) model.Client
		body      string
		wantIn    int
		wantRead  int
		wantWrite int
	}{
		{
			name:   "openai",
			client: newOpenAIForTest,
			body: `{"model":"gpt-5.6","status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":"ok"}]}],` +
				`"usage":{"input_tokens":90000,"output_tokens":5,"input_tokens_details":{"cached_tokens":1000,"cache_write_tokens":80000},"output_tokens_details":{"reasoning_tokens":0}}}`,
			wantIn: 90000, wantRead: 1000, wantWrite: 80000,
		},
		{
			name:   "openai-compatible broker",
			client: newVLLMForTest,
			body: `{"model":"anthropic/claude-haiku-4.5","choices":[{"message":{"content":"ok"},"finish_reason":"stop"}],` +
				`"usage":{"prompt_tokens":12610,"completion_tokens":4,"prompt_tokens_details":{"cached_tokens":0,"cache_write_tokens":12601}}}`,
			wantIn: 12610, wantRead: 0, wantWrite: 12601,
		},
		{
			name:   "ollama",
			client: newOllamaForTest,
			body: `{"model":"gemma3","message":{"content":"ok"},"done":true,"done_reason":"stop",` +
				`"prompt_eval_count":3000,"prompt_eval_cached_count":2800,"eval_count":4}`,
			wantIn: 3000, wantRead: 2800, wantWrite: 0,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := tc.client(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/show" {
					// An Ollama adapter may ask for the model's details first;
					// that request is not the call under test.
					_, _ = io.WriteString(w, `{}`)
					return
				}
				_, _ = io.WriteString(w, tc.body)
			})
			resp, err := client.Complete(context.Background(), model.Request{Messages: []model.Message{{Role: "user", Content: "hi"}}})
			if err != nil {
				t.Fatal(err)
			}
			if resp.InputTokens != tc.wantIn || resp.CachedTokens != tc.wantRead || resp.CacheWriteTokens != tc.wantWrite {
				t.Fatalf("in/read/write = %d/%d/%d, want %d/%d/%d",
					resp.InputTokens, resp.CachedTokens, resp.CacheWriteTokens, tc.wantIn, tc.wantRead, tc.wantWrite)
			}
		})
	}
}

// A request that set no ceiling gets the same one on the OpenAI-compatible
// wire as on every other, rather than an omitted max_tokens a host reads as the
// model's own limit.
func TestTheOpenAICompatibleWireSendsTheSharedCeilingWhenNoneWasSet(t *testing.T) {
	for _, tc := range []struct {
		name string
		set  int
		want int
	}{
		{"unset", 0, unsetMaxOutputTokens},
		{"set", 300, 300},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var wire struct {
				MaxTokens int `json:"max_tokens"`
			}
			client := newVLLMForTest(t, func(w http.ResponseWriter, r *http.Request) {
				if err := json.Unmarshal(readBody(t, r.Body), &wire); err != nil {
					t.Errorf("wire not JSON: %v", err)
				}
				_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}]}`)
			})
			if _, err := client.Complete(context.Background(), model.Request{
				MaxTokens: tc.set, Messages: []model.Message{{Role: "user", Content: "hi"}},
			}); err != nil {
				t.Fatal(err)
			}
			if wire.MaxTokens != tc.want {
				t.Fatalf("max_tokens = %d, want %d", wire.MaxTokens, tc.want)
			}
		})
	}
}
