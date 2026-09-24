// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The routing surface's wire carries no upstream preferences, so an admin who
// reads the binding and writes it straight back sends none. What the store
// holds afterwards is the claim, and only a real settings row can show it.
//
// In package compose because the handlers are unexported and the claim is about
// what THEY do with a document the contract cannot express a pin in.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// pinnedBrokerRouting binds every lane to the broker with an EU residency pin.
const pinnedBrokerRouting = `profile: eu_hosted
tiers:
  local_small: {provider: openai_compatible, model: mistralai/ministral-8b-2512, base_url: 'https://openrouter.ai/api', routing: {only: [mistral/eu]}}
  cheap_cloud: {provider: openai_compatible, model: mistralai/ministral-8b-2512, base_url: 'https://openrouter.ai/api', routing: {only: [mistral/eu]}}
  premium: {provider: openai_compatible, model: mistralai/mistral-small-2603, base_url: 'https://openrouter.ai/api', routing: {only: [mistral/eu]}}
  frontier: {provider: openai_compatible, model: mistralai/mistral-small-2603, base_url: 'https://openrouter.ai/api', routing: {only: [mistral/eu]}}
embeddings: {provider: openai_compatible, model: mistralai/mistral-embed-2312, base_url: 'https://openrouter.ai/api', dimensions: 1024, routing: {only: [mistral/eu]}}
`

func routingAdmin(e *integration.Env) context.Context {
	return e.As(e.AdminUser, nil, principal.Permissions{
		Objects:  map[string]principal.ObjectGrant{"ai_routing": {Read: true, Update: true}},
		RowScope: principal.RowScopeAll,
	})
}

// getThenPut reads the binding through GET and writes the body it answered
// back through PUT, conditioned on the ETag it answered, after edit rewrites
// the document the way a form would.
func getThenPut(ctx context.Context, t *testing.T, h aiRoutingHandlers, edit func(string) string) *httptest.ResponseRecorder {
	t.Helper()
	got := httptest.NewRecorder()
	h.GetAiRouting(got, httptest.NewRequest(http.MethodGet, "/v1/ai/routing", nil).WithContext(ctx))
	if got.Code != http.StatusOK {
		t.Fatalf("GET = %d: %s", got.Code, got.Body)
	}
	req := httptest.NewRequest(http.MethodPut, "/v1/ai/routing", strings.NewReader(edit(got.Body.String()))).WithContext(ctx)
	req.Header.Set("If-Match", got.Header().Get("ETag"))
	put := httptest.NewRecorder()
	h.ReplaceAiRouting(put, req)
	return put
}

func TestReadingTheBindingAndWritingItBackKeepsEveryResidencyPin(t *testing.T) {
	e := integration.Setup(t)
	ctx := routingAdmin(e)
	store := ai.NewRoutingStore(NewSettingsStore(e.Pool), config.Static(nil))
	pinned, err := ai.ParseRouting([]byte(pinnedBrokerRouting))
	if err != nil {
		t.Fatalf("the planted binding does not parse: %v", err)
	}
	if _, err := store.Replace(ctx, pinned); err != nil {
		t.Fatalf("storing the planted binding: %v", err)
	}

	put := getThenPut(ctx, t, aiRoutingHandlers{store: store}, func(body string) string { return body })
	if put.Code != http.StatusOK {
		t.Fatalf("PUT of the document GET answered = %d: %s", put.Code, put.Body)
	}

	stored, err := store.Get(ctx)
	if err != nil {
		t.Fatalf("reading the binding back: %v", err)
	}
	lanes := map[string]ai.ProviderConfig{"embeddings": stored.Embeddings.ProviderConfig}
	for tier, binding := range stored.Tiers {
		lanes[string(tier)] = binding
	}
	for lane, binding := range lanes {
		if binding.Routing == nil || !slices.Equal(binding.Routing.Only, []string{"mistral/eu"}) {
			t.Errorf("%s routing = %+v after GET → PUT, want the stored only: [mistral/eu] kept", lane, binding.Routing)
		}
	}
}

// A pin names hosts that serve ONE model, so re-pointing a lane at another
// model does not carry it: under eu_hosted the write is then refused, naming
// the residency rule, rather than stored pinned to hosts that cannot serve it.
func TestRepointingAPinnedLaneAtAnotherModelDoesNotCarryItsPin(t *testing.T) {
	e := integration.Setup(t)
	ctx := routingAdmin(e)
	store := ai.NewRoutingStore(NewSettingsStore(e.Pool), config.Static(nil))
	pinned, err := ai.ParseRouting([]byte(pinnedBrokerRouting))
	if err != nil {
		t.Fatalf("the planted binding does not parse: %v", err)
	}
	if _, err := store.Replace(ctx, pinned); err != nil {
		t.Fatalf("storing the planted binding: %v", err)
	}

	put := getThenPut(ctx, t, aiRoutingHandlers{store: store}, func(body string) string {
		return strings.Replace(body, "mistralai/mistral-small-2603", "openai/gpt-oss-120b", 1)
	})
	if put.Code != http.StatusUnprocessableEntity || !strings.Contains(put.Body.String(), "under profile eu_hosted") {
		t.Fatalf("PUT = %d %s, want 422 naming the eu_hosted residency rule", put.Code, put.Body)
	}
}
