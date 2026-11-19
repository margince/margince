// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"regexp"
	"slices"
	"testing"
)

// Gemini on Vertex AI serves Gemini's models, so its sheet falls back to
// Gemini's; no other provider borrows a price.
func TestOnlyVertexIsPricedByAnotherProvider(t *testing.T) {
	from, to := rateFallbacks()
	if !slices.Equal(from, []string{providerGeminiVertex}) || !slices.Equal(to, []string{providerGemini}) {
		t.Errorf("rateFallbacks = %v → %v, want gemini_vertex → gemini", from, to)
	}
	if got := pricedByFor(providerGeminiVertex); got != providerGemini {
		t.Errorf("pricedByFor(gemini_vertex) = %q, want gemini", got)
	}
	if got := pricedByFor(providerOpenAI); got != "" {
		t.Errorf("pricedByFor(openai) = %q, want none", got)
	}
}

// CostReport prices calls alike in rateKeyColumns and the day at one rate. A
// column pricedRate reads outside that key would price two unlike calls alike.
func TestTheRateKeyHoldsEveryCallColumnPricedRateReads(t *testing.T) {
	key := append(slices.Clone(rateKeyColumns), "occurred_at")
	reads := regexp.MustCompile(`\bk\.(\w+)`).FindAllStringSubmatch(pricedRate("k", 1, 2), -1)
	if len(reads) == 0 {
		t.Fatal("pricedRate(\"k\") reads no k column; the pattern no longer matches its SQL")
	}
	for _, read := range reads {
		if !slices.Contains(key, read[1]) {
			t.Errorf("pricedRate reads k.%s, which the rate key %v leaves out", read[1], key)
		}
	}
}
