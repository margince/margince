// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The serving editor saves only on a clean preview, and shows what the tier
// will send. Both answers come from the handlers over a real settings row.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// brokerRouting binds two tiers to the broker, the connection holding zero
// data retention.
const brokerRouting = `profile: cloud_frontier
providers:
  openai_compatible: {base_url: 'https://openrouter.ai/api', upstream: {provider: {zdr: true}}}
tiers:
  cheap_cloud: {provider: openai_compatible, model: openai/gpt-oss-120b}
  premium: {provider: openai_compatible, model: openai/gpt-oss-120b}
embeddings: {provider: openai_compatible, model: mistralai/mistral-embed-2312, dimensions: 1024}
`

type previewHarness struct {
	ctx     context.Context
	routing aiRoutingHandlers
	admin   aiAdminHandlers
	e       *integration.Env
}

func newPreviewHarness(t *testing.T) previewHarness {
	t.Helper()
	e := integration.Setup(t)
	ctx := e.As(e.AdminUser, nil, principal.Permissions{
		Objects:  map[string]principal.ObjectGrant{"ai_routing": {Read: true, Update: true}, "ai_budget": {Read: true}},
		RowScope: principal.RowScopeAll,
	})
	settingsStore := NewSettingsStore(e.Pool)
	store := ai.NewRoutingStore(settingsStore, config.Static(nil))
	planted, err := ai.ParseRouting([]byte(brokerRouting))
	if err != nil {
		t.Fatalf("the planted binding does not parse: %v", err)
	}
	if _, err := store.Replace(ctx, planted); err != nil {
		t.Fatalf("storing the planted binding: %v", err)
	}
	return previewHarness{
		ctx: ctx, e: e, routing: aiRoutingHandlers{store: store},
		admin: aiAdminHandlers{store: ai.NewAdminStore(e.DB(), settingsStore, budgetFullUsers, aiDeferredWork(e.Pool))},
	}
}

// documentWithTier is the document GET answers with one tier's routing
// replaced by raw, exactly as the editor sends it.
func (h previewHarness) documentWithTier(t *testing.T, tier, raw string) (body, etag string) {
	t.Helper()
	got := httptest.NewRecorder()
	h.routing.GetAiRouting(got, httptest.NewRequest(http.MethodGet, "/v1/ai/routing", nil).WithContext(h.ctx))
	if got.Code != http.StatusOK {
		t.Fatalf("GET = %d: %s", got.Code, got.Body)
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(got.Body.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	var tiers map[string]map[string]json.RawMessage
	if err := json.Unmarshal(doc["tiers"], &tiers); err != nil {
		t.Fatal(err)
	}
	tiers[tier]["routing"] = json.RawMessage(raw)
	encoded, err := json.Marshal(tiers)
	if err != nil {
		t.Fatal(err)
	}
	doc["tiers"] = encoded
	out, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	return string(out), got.Header().Get("ETag")
}

func (h previewHarness) preview(t *testing.T, body string) crmcontracts.AiRoutingPreview {
	t.Helper()
	rec := httptest.NewRecorder()
	h.admin.PreviewAiRouting(rec, httptest.NewRequest(http.MethodPost, "/v1/ai/routing/preview", strings.NewReader(body)).WithContext(h.ctx))
	if rec.Code != http.StatusOK {
		t.Fatalf("preview = %d: %s", rec.Code, rec.Body)
	}
	var out crmcontracts.AiRoutingPreview
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func (h previewHarness) auditRows(t *testing.T) int {
	t.Helper()
	var n int
	if err := h.e.Pool.QueryRow(context.Background(), `SELECT count(*) FROM audit_log`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestPreviewListsEveryBadPathAndSavesNothing(t *testing.T) {
	h := newPreviewHarness(t)
	body, etag := h.documentWithTier(t, "cheap_cloud", `{"provider":{"sort":{"by":"fastest"},"only":["groq"]}}`)
	audited := h.auditRows(t)

	got := h.preview(t, body)

	var paths []string
	if got.Errors != nil {
		for _, e := range *got.Errors {
			paths = append(paths, e.Field)
		}
	}
	slices.Sort(paths)
	want := []string{"tiers.cheap_cloud.routing.provider.only", "tiers.cheap_cloud.routing.provider.sort.by"}
	if !slices.Equal(paths, want) {
		t.Fatalf("paths %v, want %v", paths, want)
	}
	if `"`+got.CurrentVersion+`"` != etag {
		t.Errorf("preview reports version %s, the document is at %s", got.CurrentVersion, etag)
	}
	if n := h.auditRows(t); n != audited {
		t.Fatalf("preview wrote %d audit rows", n-audited)
	}
}

func TestPreviewShowsTheWireTheTierWillSend(t *testing.T) {
	h := newPreviewHarness(t)
	body, _ := h.documentWithTier(t, "cheap_cloud", `{"provider":{"sort":"latency"}}`)

	got := h.preview(t, body)

	if got.Errors != nil {
		t.Fatalf("a valid draft was refused: %+v", *got.Errors)
	}
	effective := map[string]json.RawMessage{}
	if got.Effective != nil {
		for tier, routing := range got.Effective.Tiers {
			raw, err := json.Marshal(routing)
			if err != nil {
				t.Fatal(err)
			}
			effective[tier] = raw
		}
	}
	// The tier declared routing, so the shipped default does not apply to it;
	// the connection's privacy reaches it all the same.
	if want := `{"provider":{"sort":"latency","zdr":true}}`; string(effective["cheap_cloud"]) != want {
		t.Errorf("cheap_cloud sends %s, want %s", effective["cheap_cloud"], want)
	}
	if want := `{"provider":{"quantizations":["fp16","bf16"],"require_parameters":true,"sort":"throughput","zdr":true}}`; string(effective["premium"]) != want {
		t.Errorf("premium sends %s, want the shipped default under the connection's privacy %s", effective["premium"], want)
	}
}

// The contract type drops a key it does not know, so the raw body is read: a
// misspelt key the broker would ignore is refused by its path.
func TestAnUnknownNestedKeyIsRefusedByItsPathOnSave(t *testing.T) {
	h := newPreviewHarness(t)
	body, etag := h.documentWithTier(t, "premium", `{"provider":{"sort_by":"price"}}`)
	req := httptest.NewRequest(http.MethodPut, "/v1/ai/routing", strings.NewReader(body)).WithContext(h.ctx)
	req.Header.Set("If-Match", etag)
	put := httptest.NewRecorder()

	h.routing.ReplaceAiRouting(put, req)

	if put.Code != http.StatusUnprocessableEntity || !strings.Contains(put.Body.String(), "tiers.premium.routing.provider.sort_by") {
		t.Fatalf("PUT = %d: %s", put.Code, put.Body)
	}
}

func TestTheRoutingSchemaIsServedToAnAdmin(t *testing.T) {
	h := newPreviewHarness(t)
	rec := httptest.NewRecorder()

	h.routing.GetAiRoutingSchema(rec, httptest.NewRequest(http.MethodGet, "/v1/ai/routing/schema", nil).WithContext(h.ctx))

	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "application/schema+json" {
		t.Fatalf("schema = %d %s", rec.Code, rec.Header().Get("Content-Type"))
	}
	var defs map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &defs); err != nil || defs["openRouterProvider"] == nil {
		t.Fatalf("the schema carries no openRouterProvider: %v", err)
	}
}
