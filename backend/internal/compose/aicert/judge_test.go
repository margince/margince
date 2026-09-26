// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert

import (
	"testing"

	"github.com/margince/margince/backend/internal/modules/ai"
)

// A judge from the candidate's own family grades its own homework whether or not
// it is the very same model, and both spellings a served identity takes — a bare
// provider name and a broker's publisher/name — must be recognised.
func TestSelfJudgedFlagsTheCandidatesOwnFamily(t *testing.T) {
	cases := []struct {
		name             string
		candidate, judge string
		want             bool
	}{
		{"identical", "gemini-3.5-flash", "gemini-3.5-flash", true},
		{"same line, different model", "gemini-3.1-pro-preview", "gemini-3.5-flash", true},
		{"same line across a broker and a direct provider", "google/gemini-3.5-flash", "gemini-3.1-flash-lite", true},
		{"same publisher, different line", "mistralai/ministral-14b-2512", "mistralai/mistral-large-2512", true},
		{"a direct line under its broker's publisher", "mistral-large-2512", "mistralai/ministral-8b-2512", true},
		{"same line under a local tag", "openai/gpt-oss-120b", "gpt-oss:20b", true},
		{"a Bedrock id against Claude's broker identity", "us.anthropic.claude-sonnet-4-5-20250929-v1:0", "anthropic/claude", true},
		{"a Bedrock id against a bare Claude", "anthropic.claude-3-haiku-20240307-v1:0", "claude-sonnet-4-6", true},
		{"different vendors", "gemini-3.1-flash-lite", "mistralai/mistral-large-2512", false},
		{"different vendors, both brokered", "openai/gpt-oss-120b", "z-ai/glm-5.2", false},
		{"empty candidate never counts as a match", "", "", false},
		{"empty judge never counts as a match", "gemini-3.5-flash", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := selfJudged(c.candidate, c.judge); got != c.want {
				t.Fatalf("selfJudged(%q, %q) = %v, want %v", c.candidate, c.judge, got, c.want)
			}
		})
	}
}

func TestCloudServedNamesOnlyNetworkHostedVendors(t *testing.T) {
	local := []string{"ollama", "vllm", ai.ProviderFake}
	for _, p := range local {
		if cloudServed(p) {
			t.Errorf("cloudServed(%q) = true, want false (local inference)", p)
		}
	}
	cloud := []string{"anthropic", "openai", "gemini", "openai_compatible"}
	for _, p := range cloud {
		if !cloudServed(p) {
			t.Errorf("cloudServed(%q) = false, want true (network-hosted)", p)
		}
	}
}

func TestCheckCapsGatesTokensAndCloudOnlyLatency(t *testing.T) {
	t.Run("within every cap", func(t *testing.T) {
		ok, failures := checkCaps(Caps{MaxTokens: 100, P95LatencyMS: 5000}, runCalls{TokensIn: 30, TokensOut: 30, LatencyMS: 1000, Provider: "anthropic"})
		if !ok || len(failures) != 0 {
			t.Fatalf("want ok with no failures, got ok=%v failures=%v", ok, failures)
		}
	})
	t.Run("max_tokens exceeded by the answer", func(t *testing.T) {
		// The answer (output minus reasoning) over budget fails the cap.
		ok, failures := checkCaps(Caps{MaxTokens: 50}, runCalls{TokensOut: 60, Provider: "anthropic"})
		if ok || len(failures) != 1 {
			t.Fatalf("want one max_tokens failure, got ok=%v failures=%v", ok, failures)
		}
	})
	t.Run("input and reasoning tokens do not count against the answer cap", func(t *testing.T) {
		// The offer_draft rich-context / tight-output case: a large fixed
		// input and heavy internal thinking, but a small answer. The cap
		// budgets the ANSWER, so a good concise draft passes regardless of
		// how big the scenario's input is or how hard the model thought.
		ok, failures := checkCaps(Caps{MaxTokens: 300}, runCalls{TokensIn: 468, TokensOut: 700, ReasoningTokens: 450, Provider: "gemini"})
		if !ok || len(failures) != 0 {
			t.Fatalf("a small answer under a large input+reasoning must pass, got ok=%v failures=%v", ok, failures)
		}
	})
	t.Run("p95 latency exceeded on a cloud provider", func(t *testing.T) {
		ok, failures := checkCaps(Caps{P95LatencyMS: 500}, runCalls{LatencyMS: 900, Provider: "anthropic"})
		if ok || len(failures) != 1 {
			t.Fatalf("want one p95 latency failure, got ok=%v failures=%v", ok, failures)
		}
	})
	t.Run("p95 latency ignored on a local provider", func(t *testing.T) {
		ok, failures := checkCaps(Caps{P95LatencyMS: 500}, runCalls{LatencyMS: 900, Provider: ai.ProviderFake})
		if !ok || len(failures) != 0 {
			t.Fatalf("a local provider's latency must never fail the cap, got ok=%v failures=%v", ok, failures)
		}
	})
}
