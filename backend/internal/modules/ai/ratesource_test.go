// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
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
