// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// flashLitePreset is a gemini_cloud-shaped binding with one flash-lite tier
// told to think at low and one left at the adapter's default.
const flashLitePreset = `profile: cloud_frontier
tiers:
  cheap_cloud: {provider: gemini, model: gemini-3.1-flash-lite, thinking_level: low}
  local_small: {provider: gemini, model: gemini-3.1-flash-lite}
embeddings: {provider: gemini, model: gemini-embedding-001}
`

// geminiTierForTest parses flashLitePreset and builds tier's client against a
// test server, so the level travels the yaml → binding → wire path production
// uses rather than a hand-set struct field.
func geminiTierForTest(t *testing.T, tier Tier, handler http.HandlerFunc) model.Client {
	t.Helper()
	cfg, err := ParseRouting([]byte(flashLitePreset))
	if err != nil {
		t.Fatalf("the preset under test does not parse: %v", err)
	}
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	binding := cfg.Tiers[tier]
	binding.BaseURL = srv.URL
	client, err := selectLocalBrain(binding, cloudKeyFor("gemini", testGeminiKey))
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func structuredAsk(opts map[string]json.RawMessage) model.Request {
	return model.Request{
		Messages:        []model.Message{{Role: "user", Content: "classify"}},
		ResponseSchema:  json.RawMessage(`{"type":"object"}`),
		MaxTokens:       400,
		ProviderOptions: opts,
	}
}

// A flash-lite tier is sent no level by default, because its own is shallower
// than low; a binding that names one is sent it, and a request's own level
// still outranks the binding's.
func TestABindingThinkingLevelReachesAFlashLiteRequest(t *testing.T) {
	medium := map[string]json.RawMessage{"gemini": json.RawMessage(`{"thinking_level":"medium"}`)}
	for name, tc := range map[string]struct {
		tier Tier
		opts map[string]json.RawMessage
		want string
	}{
		"binding names low":            {TierCheapCloud, nil, `"thinkingConfig":{"thinkingLevel":"low"}`},
		"binding names none":           {TierLocalSmall, nil, ""},
		"request outranks the binding": {TierCheapCloud, medium, `"thinkingConfig":{"thinkingLevel":"medium"}`},
	} {
		t.Run(name, func(t *testing.T) {
			var body []byte
			client := geminiTierForTest(t, tc.tier, func(w http.ResponseWriter, r *http.Request) {
				body = readBody(t, r.Body)
				_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"{}"}]},"finishReason":"STOP"}]}`))
			})
			if _, err := client.Complete(context.Background(), structuredAsk(tc.opts)); err != nil {
				t.Fatal(err)
			}
			if tc.want == "" {
				if bytes.Contains(body, []byte(`"thinkingConfig"`)) {
					t.Fatalf("a tier naming no level sent one: %s", body)
				}
				return
			}
			if !bytes.Contains(body, []byte(tc.want)) {
				t.Fatalf("wire lacks %s: %s", tc.want, body)
			}
		})
	}
}

// Thinking a tier was told to do is charged like any other output, and when it
// eats the ceiling the retry grows the room rather than asking for brevity —
// with the binding's level still on the retried request.
func TestThinkingThatSpendsTheCeilingIsReportedAndEarnsRoom(t *testing.T) {
	var bodies [][]byte
	client := geminiTierForTest(t, TierCheapCloud, func(w http.ResponseWriter, r *http.Request) {
		bodies = append(bodies, readBody(t, r.Body))
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"{\"a\""}]},"finishReason":"MAX_TOKENS"}],
			"usageMetadata":{"promptTokenCount":50,"candidatesTokenCount":5,"thoughtsTokenCount":380}}`))
	})
	req := structuredAsk(nil)
	resp, err := client.Complete(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.ReasoningTokens != 380 || resp.OutputTokens != 385 {
		t.Fatalf("reasoning=%d output=%d, want 380 reported and charged inside 385", resp.ReasoningTokens, resp.OutputTokens)
	}
	retry := feedbackFor(req, resp, nil)
	if retry.MaxTokens <= req.MaxTokens {
		t.Fatalf("retry ceiling %d, want it raised above %d for the thinking that spent it", retry.MaxTokens, req.MaxTokens)
	}
	if _, err := client.Complete(context.Background(), retry); err != nil {
		t.Fatal(err)
	}
	if len(bodies) != 2 || !bytes.Contains(bodies[1], []byte(`"thinkingLevel":"low"`)) {
		t.Fatalf("the retried request lost the binding's level: %s", bodies[len(bodies)-1])
	}
}

// A level no request could carry is refused when the config loads, with a
// message naming the lane and the edit.
func TestAThinkingLevelNoRequestCouldCarryIsRefusedAtLoad(t *testing.T) {
	tiered := func(binding, embeddings string) string {
		return "profile: cloud_frontier\ntiers:\n  premium: {" + binding + "}\nembeddings: {" + embeddings + "}\n"
	}
	const embed = "provider: gemini, model: gemini-embedding-001"
	for name, tc := range map[string]struct {
		yaml, refusal string
	}{
		"an unknown level": {
			tiered("provider: gemini, model: gemini-3.1-flash-lite, thinking_level: lots", embed),
			`tier premium: thinking_level "lots" is not one of minimal | low | medium | high`,
		},
		"a provider without the field": {
			tiered("provider: openai_compatible, base_url: https://api.mistral.ai, model: m, thinking_level: low", embed),
			"tier premium: `thinking_level` is Gemini's thinkingConfig and provider openai_compatible has no such field",
		},
		"a model that predates the field": {
			tiered("provider: gemini, model: gemini-2.5-flash, thinking_level: low", embed),
			"model gemini-2.5-flash predates thinkingLevel",
		},
		"the embeddings lane": {
			tiered("provider: gemini, model: gemini-3.5-flash", embed+", thinking_level: low"),
			"the embeddings lane takes no `thinking_level`",
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := ParseRouting([]byte(tc.yaml))
			if err == nil || !strings.Contains(err.Error(), tc.refusal) {
				t.Fatalf("err = %v, want a refusal saying %q", err, tc.refusal)
			}
		})
	}
}
