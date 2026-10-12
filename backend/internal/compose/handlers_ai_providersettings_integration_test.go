// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// PUT /ai/provider-settings/{provider} against a real settings row: what the
// handler answers, and what the next GET then shows every lane.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/platform/config"
)

func putProviderSettings(t *testing.T, h aiRoutingHandlers, e *integration.Env, provider, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPut, "/v1/ai/provider-settings/"+provider, strings.NewReader(body)).WithContext(routingAdmin(e))
	rec := httptest.NewRecorder()
	h.SetAiProviderSettings(rec, req, provider)
	return rec
}

func TestSettingAProvidersHostRepointsEveryLaneOnIt(t *testing.T) {
	e := integration.Setup(t)
	store := ai.NewRoutingStore(NewSettingsStore(e.Pool), config.Static(nil))
	h := aiRoutingHandlers{store: store}
	planted, err := ai.ParseRouting([]byte(pinnedBrokerRouting))
	if err != nil {
		t.Fatalf("the planted binding does not parse: %v", err)
	}
	if _, err := store.Replace(routingAdmin(e), planted); err != nil {
		t.Fatalf("storing the planted binding: %v", err)
	}

	rec := putProviderSettings(t, h, e, "openai_compatible", `{"base_url":"https://eu.openrouter.ai/api","upstream":{"only":["mistral/eu"]}}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	if rec.Header().Get("ETag") == "" {
		t.Error("no ETag: a client cannot condition its next routing write on this one")
	}
	var got crmcontracts.AiRouting
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decoding the answer: %v", err)
	}
	if len(got.Tiers) == 0 {
		t.Fatal("the answer carries no tiers, so nothing below checks the new host reached a lane")
	}
	for name, tier := range got.Tiers {
		if tier.BaseUrl == nil || *tier.BaseUrl != "https://eu.openrouter.ai/api" {
			t.Errorf("tier %s base_url = %v, want the provider's new host", name, tier.BaseUrl)
		}
	}
}

func TestClearingAHostALaneStillNeedsIsRefusedNamingTheLanes(t *testing.T) {
	e := integration.Setup(t)
	store := ai.NewRoutingStore(NewSettingsStore(e.Pool), config.Static(nil))
	planted, err := ai.ParseRouting([]byte(pinnedBrokerRouting))
	if err != nil {
		t.Fatalf("the planted binding does not parse: %v", err)
	}
	if _, err := store.Replace(routingAdmin(e), planted); err != nil {
		t.Fatalf("storing the planted binding: %v", err)
	}

	rec := putProviderSettings(t, aiRoutingHandlers{store: store}, e, "openai_compatible", `{}`)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422: %s", rec.Code, rec.Body)
	}
	if body := rec.Body.String(); !strings.Contains(body, ai.CodeNoHost) || !strings.Contains(body, "tier premium") {
		t.Errorf("refusal = %s, want code %s naming the lanes that bind the provider", body, ai.CodeNoHost)
	}
}

func TestAProviderThisBuildDoesNotKnowIsNotFound(t *testing.T) {
	e := integration.Setup(t)
	h := aiRoutingHandlers{store: ai.NewRoutingStore(NewSettingsStore(e.Pool), config.Static(nil))}

	rec := putProviderSettings(t, h, e, "no_such_vendor", `{"base_url":"https://x.example"}`)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404: %s", rec.Code, rec.Body)
	}
}

// The answer to a write is the document as stored: its ETag is the one the next
// GET answers, so a client may send it straight back as If-Match, and its tier
// routing is what was written, so sending the body back freezes nothing.
func TestAWritesAnswerIsTheDocumentGETReadsBack(t *testing.T) {
	e := integration.Setup(t)
	store := ai.NewRoutingStore(NewSettingsStore(e.Pool), config.Static(nil))
	h := aiRoutingHandlers{store: store}
	planted, err := ai.ParseRouting([]byte(pinnedBrokerRouting))
	if err != nil {
		t.Fatalf("the planted binding does not parse: %v", err)
	}
	if _, err := store.Replace(routingAdmin(e), planted); err != nil {
		t.Fatalf("storing the planted binding: %v", err)
	}

	put := putProviderSettings(t, h, e, "openai_compatible", `{"base_url":"https://openrouter.ai/api","upstream":{"only":["mistral/eu"]}}`)
	if put.Code != http.StatusOK {
		t.Fatalf("PUT provider settings = %d: %s", put.Code, put.Body)
	}
	got := httptest.NewRecorder()
	h.GetAiRouting(got, httptest.NewRequest(http.MethodGet, "/v1/ai/routing", nil).WithContext(routingAdmin(e)))
	if put.Header().Get("ETag") != got.Header().Get("ETag") {
		t.Errorf("PUT answered ETag %s, the next GET %s", put.Header().Get("ETag"), got.Header().Get("ETag"))
	}
	if put.Body.String() != got.Body.String() {
		t.Errorf("PUT answered\n%s\nthe next GET\n%s", put.Body, got.Body)
	}

	req := httptest.NewRequest(http.MethodPut, "/v1/ai/routing", strings.NewReader(put.Body.String())).WithContext(routingAdmin(e))
	req.Header.Set("If-Match", put.Header().Get("ETag"))
	again := httptest.NewRecorder()
	h.ReplaceAiRouting(again, req)
	if again.Code != http.StatusOK {
		t.Fatalf("writing back the answer with its own ETag = %d: %s", again.Code, again.Body)
	}
	if again.Header().Get("ETag") != put.Header().Get("ETag") {
		t.Error("writing the answer back unchanged moved the revision: it froze something into the document")
	}
}
