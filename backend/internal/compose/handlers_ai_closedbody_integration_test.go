// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The routing document's contract closes every nested object, so a key it does
// not declare is a 422 naming its path, never a 200 for a setting not stored.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestAnUnknownNestedKeyIsRefusedNamingItsPathOnEveryRoutingWrite(t *testing.T) {
	e := integration.Setup(t)
	settings := NewSettingsStore(e.Pool)
	store := ai.NewRoutingStore(settings, config.Static(nil))
	routing := aiRoutingHandlers{store: store}
	admin := aiAdminHandlers{store: ai.NewAdminStore(e.DB(), settings, budgetFullUsers, aiDeferredWork(e.Pool))}
	ctx := e.As(e.AdminUser, nil, principal.Permissions{
		Objects:  map[string]principal.ObjectGrant{"ai_routing": {Read: true, Update: true}, "ai_budget": {Read: true}},
		RowScope: principal.RowScopeAll,
	})

	cases := map[string]struct {
		serve func(w http.ResponseWriter, r *http.Request)
		url   string
		body  string
		path  string
	}{
		"provider settings upstream": {
			func(w http.ResponseWriter, r *http.Request) { routing.SetAiProviderSettings(w, r, "openai_compatible") },
			"/v1/ai/provider-settings/openai_compatible",
			`{"base_url":"https://openrouter.ai/api","upstream":{"only":["mistral/eu"],"order":["a"]}}`,
			`upstream.order`,
		},
		"routing preview tier binding": {
			admin.PreviewAiRouting, "/v1/ai/routing/preview",
			`{"tiers":{"cheap_cloud":{"provider":"fake","model":"m","nope":1}}}`,
			`tiers.cheap_cloud.nope`,
		},
		"routing preview providers entry": {
			admin.PreviewAiRouting, "/v1/ai/routing/preview",
			`{"providers":{"openai_compatible":{"base_url":"https://openrouter.ai/api","nope":1}}}`,
			`providers.openai_compatible.nope`,
		},
		"routing replace tier routing": {
			routing.ReplaceAiRouting, "/v1/ai/routing",
			`{"tiers":{"cheap_cloud":{"provider":"fake","model":"m","routing":{"provider":{"zdr_typo":true}}}}}`,
			`tiers.cheap_cloud.routing.provider.zdr_typo`,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			method := http.MethodPut
			if strings.HasSuffix(tc.url, "/preview") {
				method = http.MethodPost
			}
			rec := httptest.NewRecorder()
			tc.serve(rec, httptest.NewRequest(method, tc.url, strings.NewReader(tc.body)).WithContext(ctx))
			if rec.Code != http.StatusUnprocessableEntity {
				t.Fatalf("status = %d, want 422: %s", rec.Code, rec.Body)
			}
			if !strings.Contains(rec.Body.String(), tc.path) {
				t.Errorf("refusal = %s, want it to name %q", rec.Body, tc.path)
			}
		})
	}
}

func TestAVertexProviderEntryWithAHostIsRefusedNamingTheEntry(t *testing.T) {
	e := integration.Setup(t)
	h := aiRoutingHandlers{store: ai.NewRoutingStore(NewSettingsStore(e.Pool), config.Static(nil))}

	rec := putProviderSettings(t, h, e, "gemini_vertex", `{"base_url":"https://x.example","location":"eu"}`)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422: %s", rec.Code, rec.Body)
	}
	if body := rec.Body.String(); !strings.Contains(body, "providers: gemini_vertex") || !strings.Contains(body, "takes no base_url") {
		t.Errorf("refusal = %s, want it to name the provider entry and the rule", body)
	}
}
