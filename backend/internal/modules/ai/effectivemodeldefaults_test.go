// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"testing"

	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// EffectiveModel answers "which model does this binding actually serve with?",
// and the client builder answers the same question when it fills a client's
// defaultModel. Two writers of one rule.
//
// The reason it matters is a screen: a settings card reporting how reliably the
// bound model performs a job resolves the model through EffectiveModel and looks
// up a certification record by it. If the builder's default and this one part
// company, the card reports on a model nothing served, or reports "no model
// selected" about a job the router runs every day.
//
// Held by: this test, which drives the real builder and reads the default off
// the client it produced, rather than comparing two constants that happen to
// agree today.
func TestEffectiveModelIsWhatTheClientBuilderWouldServe(t *testing.T) {
	t.Parallel()

	// The providers that HAVE a default: a binding may omit the model and still
	// be served, which is the whole reason EffectiveModel exists.
	for _, provider := range []string{providerOllama, providerVLLM} {
		t.Run(provider, func(t *testing.T) {
			t.Parallel()

			binding := ProviderConfig{Provider: provider}
			client, err := SelectBrain(binding, config.Lookup(func(string) string { return "" }))
			if err != nil {
				t.Fatalf("building a client for %s with no model: %v", provider, err)
			}
			served := builderDefaultModel(t, client)
			if got := EffectiveModel(binding); got != served {
				t.Errorf("EffectiveModel = %q but the builder serves %q — the card would report "+
					"on a model nothing answered with", got, served)
			}
		})
	}
}

// An explicit model is never overridden by a default, for either kind of
// caller: the operator's choice is the answer.
func TestAnExplicitModelSurvivesBothWriters(t *testing.T) {
	t.Parallel()

	for _, provider := range []string{providerOllama, providerVLLM} {
		t.Run(provider, func(t *testing.T) {
			t.Parallel()

			binding := ProviderConfig{Provider: provider, Model: "operator/chose-this"}
			client, err := SelectBrain(binding, config.Lookup(func(string) string { return "" }))
			if err != nil {
				t.Fatalf("building a client for %s: %v", provider, err)
			}
			if served := builderDefaultModel(t, client); served != binding.Model {
				t.Errorf("the builder serves %q over the operator's %q", served, binding.Model)
			}
			if got := EffectiveModel(binding); got != binding.Model {
				t.Errorf("EffectiveModel = %q, want the operator's own %q", got, binding.Model)
			}
		})
	}
}

// builderDefaultModel reads the model a built client would send when a request
// names none. Reached through the concrete types on purpose: the point is what
// the BUILDER decided, and an interface method would report whatever the client
// chose to expose instead.
func builderDefaultModel(t *testing.T, client model.Client) string {
	t.Helper()
	switch c := client.(type) {
	case *ollamaClient:
		return c.defaultModel
	case *openAICompatClient:
		return c.defaultModel
	default:
		t.Fatalf("unrecognized client type %T; this test must be taught the new adapter's "+
			"default rather than silently stop checking it", client)
		return ""
	}
}
