// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func providerHealthRequest(grants map[string]principal.ObjectGrant) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/v1/ai/provider-health", nil)
	return r.WithContext(principal.WithActor(r.Context(), principal.Principal{
		Type:        principal.PrincipalHuman,
		Permissions: principal.Permissions{Objects: grants},
	}))
}

func TestProviderHealthListsOnlyBlockedProvidersInNameOrderWithoutProviderText(t *testing.T) {
	t.Parallel()
	book := newProviderBook(newClock().now)
	book.tracker("openai").observe(admission{}, ErrProviderQuota)
	book.tracker("anthropic").observe(admission{}, ErrProviderUnauthorized)
	book.tracker("gemini").observe(admission{}, nil)
	h := Handlers{providers: book}

	w := httptest.NewRecorder()
	h.GetAiProviderHealth(w, providerHealthRequest(map[string]principal.ObjectGrant{"ai_diagnostics": {Read: true}}))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", w.Code, w.Body.String())
	}
	var got crmcontracts.AiProviderHealth
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	if len(got.Providers) != 2 ||
		got.Providers[0].Provider != "anthropic" || got.Providers[0].Health != crmcontracts.AiProviderHealthEntryHealthUnauthorized ||
		got.Providers[1].Provider != "openai" || got.Providers[1].Health != crmcontracts.AiProviderHealthEntryHealthOutOfCredit {
		t.Fatalf("providers = %+v, want anthropic unauthorized then openai out_of_credit", got.Providers)
	}
	if got.Providers[0].RetryAfter == nil || got.Providers[0].Since.IsZero() {
		t.Errorf("a blocked provider must say since when and when one probe is allowed: %+v", got.Providers[0])
	}
}

func TestProviderHealthIsAnEmptyListWhenEveryProviderAnswers(t *testing.T) {
	t.Parallel()
	w := httptest.NewRecorder()
	Handlers{providers: newProviderBook(newClock().now)}.GetAiProviderHealth(
		w, providerHealthRequest(map[string]principal.ObjectGrant{"ai_diagnostics": {Read: true}}))
	if !strings.Contains(w.Body.String(), `"providers":[]`) {
		t.Errorf("want an empty providers array, got %s", w.Body.String())
	}
}

func TestProviderHealthRequiresTheDiagnosticsGrant(t *testing.T) {
	t.Parallel()
	w := httptest.NewRecorder()
	Handlers{providers: newProviderBook(newClock().now)}.GetAiProviderHealth(w, providerHealthRequest(nil))
	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403: %s", w.Code, w.Body.String())
	}
}

// Healthy and unconfigured both list no provider, and only `configured` tells
// the reader which of the two they are looking at.
func TestProviderHealthTellsNoProviderConfiguredFromEveryProviderAnswering(t *testing.T) {
	t.Parallel()
	grants := map[string]principal.ObjectGrant{"ai_diagnostics": {Read: true}}
	read := func(h Handlers) crmcontracts.AiProviderHealth {
		w := httptest.NewRecorder()
		h.GetAiProviderHealth(w, providerHealthRequest(grants))
		var got crmcontracts.AiProviderHealth
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
			t.Fatalf("decoding %s: %v", w.Body.String(), err)
		}
		return got
	}

	unconfigured := read(Handlers{providers: newProviderBook(newClock().now)})
	if unconfigured.Configured == nil || len(unconfigured.Configured) != 0 || len(unconfigured.Providers) != 0 {
		t.Errorf("no routing bound answered %+v, want an empty configured list beside an empty providers list", unconfigured)
	}

	bound := Handlers{providers: newProviderBook(newClock().now)}.WithBoundModels(func() map[Tier]ModelRef {
		return map[Tier]ModelRef{
			"cloud_large":  {Provider: "openai", Model: "gpt"},
			"cloud_small":  {Provider: "anthropic", Model: "haiku"},
			"local_small":  {Provider: "openai", Model: "mini"},
			TierEmbedLane:  {Provider: "ollama", Model: "nomic"},
			TierDecideLane: {},
		}
	})
	healthy := read(bound)
	if want := []string{"anthropic", "ollama", "openai"}; !slices.Equal(healthy.Configured, want) {
		t.Errorf("configured = %v, want each bound provider once, in name order: %v", healthy.Configured, want)
	}
	if len(healthy.Providers) != 0 {
		t.Errorf("providers = %+v, want none listed while every bound provider answers", healthy.Providers)
	}
}
