// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// happyCatalogueBody carries: a scored model and its higher-scoring-tied
// batch alias (dedupe must keep the bare id), a model with no benchmark at
// all (must be excluded), a scored model whose prompt price is scientific
// notation (must be dropped, not rendered broken), and a second real model.
const happyCatalogueBody = `{"data":[
	{"id":"anthropic/claude-opus-5","name":"Claude Opus 5","context_length":200000,
	 "pricing":{"prompt":"0.00000015","completion":"0.00000075"},
	 "benchmarks":{"artificial_analysis":{"intelligence_index":63.1}}},
	{"id":"anthropic/claude-opus-5:batch","name":"Claude Opus 5 (batch)","context_length":200000,
	 "pricing":{"prompt":"0.000000075","completion":"0.000000375"},
	 "benchmarks":{"artificial_analysis":{"intelligence_index":63.1}}},
	{"id":"vendor/no-benchmark","name":"No Benchmark",
	 "pricing":{"prompt":"0.000001","completion":"0.000002"}},
	{"id":"vendor/bad-price","name":"Bad Price",
	 "pricing":{"prompt":"1e-7","completion":"0.000002"},
	 "benchmarks":{"artificial_analysis":{"intelligence_index":50.0}}},
	{"id":"vendor/second","name":"Second Model",
	 "pricing":{"prompt":"0.000001","completion":"0.000002"},
	 "benchmarks":{"artificial_analysis":{"intelligence_index":40.5}}}
]}`

// fakeCatalogueFetcher is the injected catalogueFetcher: it never touches
// the network, counts how many times it was asked, and returns either a
// fixed body or a fixed error.
type fakeCatalogueFetcher struct {
	body  []byte
	err   error
	calls int
}

func (f *fakeCatalogueFetcher) Fetch(context.Context) ([]byte, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return f.body, nil
}

// fixedClock is a Clock that only advances when the test tells it to, so
// the 15-minute TTL is provable without sleeping one.
type fixedClock struct{ now time.Time }

func (c *fixedClock) Now() time.Time { return c.now }

func newTestCatalogue(fetcher *fakeCatalogueFetcher, clock *fixedClock) *ModelCatalogue {
	return &ModelCatalogue{
		fetcher: fetcher, clock: clock,
		cache: map[string]crmcontracts.AiModelCatalogueResponse{},
	}
}

func humanCtx() context.Context {
	return principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:test",
	})
}

func TestModelCatalogueListRanksDedupesAndPrices(t *testing.T) {
	fetcher := &fakeCatalogueFetcher{body: []byte(happyCatalogueBody)}
	clock := &fixedClock{now: time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)}
	resp := newTestCatalogue(fetcher, clock).List(context.Background(), "openrouter")

	if resp.Unavailable {
		t.Fatalf("got Unavailable, want a served catalogue")
	}
	if len(resp.Data) != 2 {
		t.Fatalf("got %d entries, want 2 (no-benchmark and bad-price excluded): %+v", len(resp.Data), resp.Data)
	}
	first := resp.Data[0]
	if first.ModelId != "anthropic/claude-opus-5" {
		t.Errorf("first entry id = %q, want the bare id kept over its :batch alias", first.ModelId)
	}
	if first.InputPerMtok != "0.15" || first.OutputPerMtok != "0.75" {
		t.Errorf("first entry prices = %s/%s, want 0.15/0.75", first.InputPerMtok, first.OutputPerMtok)
	}
	if first.RankScore == nil || *first.RankScore != "63.1" {
		t.Errorf("first entry rank score = %v, want 63.1", first.RankScore)
	}
	second := resp.Data[1]
	if second.ModelId != "vendor/second" || second.InputPerMtok != "1" || second.OutputPerMtok != "2" {
		t.Errorf("second entry = %+v, want vendor/second priced 1/2", second)
	}
	if resp.RankedBy != catalogueRankedBy {
		t.Errorf("ranked_by = %q, want %q", resp.RankedBy, catalogueRankedBy)
	}
	if resp.FetchedAt == nil || !resp.FetchedAt.Equal(clock.now) {
		t.Errorf("fetched_at = %v, want %v", resp.FetchedAt, clock.now)
	}
}

func TestModelCatalogueListFailsOpenOnNon200(t *testing.T) {
	fetcher := &fakeCatalogueFetcher{err: errors.New("openrouter answered 500")}
	resp := newTestCatalogue(fetcher, &fixedClock{}).List(context.Background(), "openrouter")
	if !resp.Unavailable || len(resp.Data) != 0 {
		t.Fatalf("got %+v, want Unavailable with an empty list", resp)
	}
}

func TestModelCatalogueListFailsOpenOnTimeout(t *testing.T) {
	fetcher := &fakeCatalogueFetcher{err: context.DeadlineExceeded}
	resp := newTestCatalogue(fetcher, &fixedClock{}).List(context.Background(), "openrouter")
	if !resp.Unavailable || len(resp.Data) != 0 {
		t.Fatalf("got %+v, want Unavailable with an empty list", resp)
	}
}

func TestModelCatalogueListFailsOpenOnUnparseableBody(t *testing.T) {
	fetcher := &fakeCatalogueFetcher{body: []byte("not json")}
	resp := newTestCatalogue(fetcher, &fixedClock{}).List(context.Background(), "openrouter")
	if !resp.Unavailable || len(resp.Data) != 0 {
		t.Fatalf("got %+v, want Unavailable with an empty list", resp)
	}
}

func TestModelCatalogueListServesTheCacheWithinTTL(t *testing.T) {
	fetcher := &fakeCatalogueFetcher{body: []byte(happyCatalogueBody)}
	clock := &fixedClock{now: time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)}
	catalogue := newTestCatalogue(fetcher, clock)

	catalogue.List(context.Background(), "openrouter")
	clock.now = clock.now.Add(14 * time.Minute)
	catalogue.List(context.Background(), "openrouter")
	if fetcher.calls != 1 {
		t.Fatalf("fetcher called %d times within the TTL, want 1", fetcher.calls)
	}

	clock.now = clock.now.Add(2 * time.Minute) // 16 minutes since the first read
	catalogue.List(context.Background(), "openrouter")
	if fetcher.calls != 2 {
		t.Fatalf("fetcher called %d times after the TTL elapsed, want 2", fetcher.calls)
	}
}

func TestModelCatalogueListNeverCachesAFailure(t *testing.T) {
	fetcher := &fakeCatalogueFetcher{err: errors.New("openrouter answered 503")}
	catalogue := newTestCatalogue(fetcher, &fixedClock{})

	catalogue.List(context.Background(), "openrouter")
	catalogue.List(context.Background(), "openrouter")
	if fetcher.calls != 2 {
		t.Fatalf("fetcher called %d times across two failures, want 2 (a failure must never be cached)", fetcher.calls)
	}
}

func TestListAiModelCatalogueRefusesAnAgentPrincipal(t *testing.T) {
	fetcher := &fakeCatalogueFetcher{body: []byte(happyCatalogueBody)}
	h := Handlers{}.WithCatalogue(newTestCatalogue(fetcher, &fixedClock{}))

	request := httptest.NewRequest(http.MethodGet, "/v1/ai-model-catalogue?provider=openrouter", nil)
	request = request.WithContext(principal.WithActor(request.Context(), principal.Principal{
		Type: principal.PrincipalAgent, ID: "agent:test",
	}))
	recorder := httptest.NewRecorder()
	h.ListAiModelCatalogue(recorder, request, crmcontracts.ListAiModelCatalogueParams{Provider: crmcontracts.Openrouter})

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", recorder.Code)
	}
	if fetcher.calls != 0 {
		t.Errorf("fetcher called %d times, want 0: a refused principal must never reach the vendor", fetcher.calls)
	}
}

func TestListAiModelCatalogueServesUnavailableWithNoCatalogueWired(t *testing.T) {
	h := Handlers{}
	request := httptest.NewRequest(http.MethodGet, "/v1/ai-model-catalogue?provider=openrouter", nil).WithContext(humanCtx())
	recorder := httptest.NewRecorder()
	h.ListAiModelCatalogue(recorder, request, crmcontracts.ListAiModelCatalogueParams{Provider: crmcontracts.Openrouter})

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 even with no catalogue source composed", recorder.Code)
	}
	var body crmcontracts.AiModelCatalogueResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if !body.Unavailable || body.Data == nil || len(body.Data) != 0 {
		t.Fatalf("got %+v, want Unavailable with an empty (non-null) list", body)
	}
}

func TestListAiModelCatalogueServesTheRankedList(t *testing.T) {
	fetcher := &fakeCatalogueFetcher{body: []byte(happyCatalogueBody)}
	h := Handlers{}.WithCatalogue(newTestCatalogue(fetcher, &fixedClock{}))

	request := httptest.NewRequest(http.MethodGet, "/v1/ai-model-catalogue?provider=openrouter", nil).WithContext(humanCtx())
	recorder := httptest.NewRecorder()
	h.ListAiModelCatalogue(recorder, request, crmcontracts.ListAiModelCatalogueParams{Provider: crmcontracts.Openrouter})

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", recorder.Code, recorder.Body.String())
	}
	var body crmcontracts.AiModelCatalogueResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if body.Unavailable || len(body.Data) != 2 {
		t.Fatalf("got %+v, want a served, two-entry catalogue", body)
	}
}
