// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"
	"errors"
	"testing"
	"time"
)

// happyCatalogueBody carries: a scored model and its higher-scoring-tied
// batch alias (dedupe must keep the bare id), a model with no benchmark at
// all (must be excluded from the ranked view, but kept in the full one), a
// scored model whose prompt price is scientific notation (must be dropped
// from the ranked view, kept priceless in the full one), and a second real
// model.
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
	return &ModelCatalogue{fetcher: fetcher, clock: clock}
}

func TestModelCatalogueListRanksDedupesAndPrices(t *testing.T) {
	fetcher := &fakeCatalogueFetcher{body: []byte(happyCatalogueBody)}
	clock := &fixedClock{now: time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)}
	resp := newTestCatalogue(fetcher, clock).List(context.Background(), 10)

	if resp.Unavailable != AvailabilityOK {
		t.Fatalf("got Unavailable %q, want a served catalogue", resp.Unavailable)
	}
	if len(resp.Models) != 2 {
		t.Fatalf("got %d entries, want 2 (no-benchmark and bad-price excluded): %+v", len(resp.Models), resp.Models)
	}
	first := resp.Models[0]
	if first.ID != "anthropic/claude-opus-5" {
		t.Errorf("first entry id = %q, want the bare id kept over its :batch alias", first.ID)
	}
	if first.InputPerMtok == nil || *first.InputPerMtok != "0.15" ||
		first.OutputPerMtok == nil || *first.OutputPerMtok != "0.75" {
		t.Errorf("first entry prices = %v/%v, want 0.15/0.75", first.InputPerMtok, first.OutputPerMtok)
	}
	if first.RankScore == nil || *first.RankScore != "63.1" {
		t.Errorf("first entry rank score = %v, want 63.1", first.RankScore)
	}
	second := resp.Models[1]
	if second.ID != "vendor/second" || second.InputPerMtok == nil || *second.InputPerMtok != "1" ||
		second.OutputPerMtok == nil || *second.OutputPerMtok != "2" {
		t.Errorf("second entry = %+v, want vendor/second priced 1/2", second)
	}
	if resp.RankedBy != catalogueRankedBy {
		t.Errorf("ranked_by = %q, want %q", resp.RankedBy, catalogueRankedBy)
	}
}

// Omitting `top` answers the vendor's whole list, unranked: the no-benchmark
// and bad-price models excluded above must both be present here, the second
// with no price rather than dropped.
func TestModelCatalogueListServesTheFullListWithNoTop(t *testing.T) {
	fetcher := &fakeCatalogueFetcher{body: []byte(happyCatalogueBody)}
	resp := newTestCatalogue(fetcher, &fixedClock{}).List(context.Background(), 0)

	if resp.Unavailable != AvailabilityOK {
		t.Fatalf("got Unavailable %q, want a served catalogue", resp.Unavailable)
	}
	if len(resp.Models) != 5 {
		t.Fatalf("got %d entries, want all 5 the vendor listed: %+v", len(resp.Models), resp.Models)
	}
	if resp.RankedBy != "" {
		t.Errorf("ranked_by = %q, want empty: an unranked list must not claim a measure", resp.RankedBy)
	}
	var badPrice AvailableModel
	found := false
	for _, m := range resp.Models {
		if m.ID == "vendor/bad-price" {
			badPrice, found = m, true
		}
	}
	if !found {
		t.Fatal("vendor/bad-price is missing from the full list; it must only be excluded from a ranked one")
	}
	if badPrice.InputPerMtok != nil {
		t.Errorf("vendor/bad-price carries a price %v, want absent: its prompt price does not parse", *badPrice.InputPerMtok)
	}
	if badPrice.RankScore != nil {
		t.Errorf("vendor/bad-price carries a rank score on an unranked list")
	}
}

func TestModelCatalogueListFailsOpenOnNon200(t *testing.T) {
	fetcher := &fakeCatalogueFetcher{err: errors.New("openrouter answered 500")}
	resp := newTestCatalogue(fetcher, &fixedClock{}).List(context.Background(), 10)
	if resp.Unavailable != AvailabilityUnreachable || len(resp.Models) != 0 {
		t.Fatalf("got %+v, want Unavailable(unreachable) with an empty list", resp)
	}
}

func TestModelCatalogueListFailsOpenOnTimeout(t *testing.T) {
	fetcher := &fakeCatalogueFetcher{err: context.DeadlineExceeded}
	resp := newTestCatalogue(fetcher, &fixedClock{}).List(context.Background(), 10)
	if resp.Unavailable != AvailabilityUnreachable || len(resp.Models) != 0 {
		t.Fatalf("got %+v, want Unavailable(unreachable) with an empty list", resp)
	}
}

func TestModelCatalogueListFailsOpenOnUnparseableBody(t *testing.T) {
	fetcher := &fakeCatalogueFetcher{body: []byte("not json")}
	resp := newTestCatalogue(fetcher, &fixedClock{}).List(context.Background(), 10)
	if resp.Unavailable != AvailabilityUnreachable || len(resp.Models) != 0 {
		t.Fatalf("got %+v, want Unavailable(unreachable) with an empty list", resp)
	}
}

func TestModelCatalogueListServesTheCacheWithinTTL(t *testing.T) {
	fetcher := &fakeCatalogueFetcher{body: []byte(happyCatalogueBody)}
	clock := &fixedClock{now: time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)}
	catalogue := newTestCatalogue(fetcher, clock)

	catalogue.List(context.Background(), 10)
	clock.now = clock.now.Add(14 * time.Minute)
	catalogue.List(context.Background(), 10)
	if fetcher.calls != 1 {
		t.Fatalf("fetcher called %d times within the TTL, want 1", fetcher.calls)
	}

	clock.now = clock.now.Add(2 * time.Minute) // 16 minutes since the first read
	catalogue.List(context.Background(), 10)
	if fetcher.calls != 2 {
		t.Fatalf("fetcher called %d times after the TTL elapsed, want 2", fetcher.calls)
	}
}

// A cached vendor payload serves BOTH a ranked and a full request without a
// second fetch: the cache holds the parse, not a capped view of it.
func TestModelCatalogueListServesEitherViewFromOneCachedFetch(t *testing.T) {
	fetcher := &fakeCatalogueFetcher{body: []byte(happyCatalogueBody)}
	catalogue := newTestCatalogue(fetcher, &fixedClock{})

	catalogue.List(context.Background(), 10)
	catalogue.List(context.Background(), 0)
	if fetcher.calls != 1 {
		t.Fatalf("fetcher called %d times for two views inside the TTL, want 1", fetcher.calls)
	}
}

func TestModelCatalogueListNeverCachesAFailure(t *testing.T) {
	fetcher := &fakeCatalogueFetcher{err: errors.New("openrouter answered 503")}
	catalogue := newTestCatalogue(fetcher, &fixedClock{})

	catalogue.List(context.Background(), 10)
	catalogue.List(context.Background(), 10)
	if fetcher.calls != 2 {
		t.Fatalf("fetcher called %d times across two failures, want 2 (a failure must never be cached)", fetcher.calls)
	}
}

// A billing-lane variant that outscores its own base model still reaches the
// screen under the id a screen can bind. The tie-break in the sort only
// prefers the bare id when the scores are EQUAL, so nothing before this
// collapse step keeps a `:batch` suffix off the ranked list.
func TestModelCatalogueRanksAnAliasUnderItsBindableID(t *testing.T) {
	const aliasScoresHigher = `{"data":[
		{"id":"anthropic/claude-opus-5","name":"Claude Opus 5","context_length":200000,
		 "pricing":{"prompt":"0.00000015","completion":"0.00000075"},
		 "benchmarks":{"artificial_analysis":{"intelligence_index":63.1}}},
		{"id":"anthropic/claude-opus-5:batch","name":"Claude Opus 5 (batch)","context_length":200000,
		 "pricing":{"prompt":"0.000000075","completion":"0.000000375"},
		 "benchmarks":{"artificial_analysis":{"intelligence_index":64.2}}}
	]}`
	fetcher := &fakeCatalogueFetcher{body: []byte(aliasScoresHigher)}
	clock := &fixedClock{now: time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)}
	resp := newTestCatalogue(fetcher, clock).List(context.Background(), 10)

	if len(resp.Models) != 1 {
		t.Fatalf("got %d entries, want the two lanes collapsed onto one: %+v", len(resp.Models), resp.Models)
	}
	if resp.Models[0].ID != "anthropic/claude-opus-5" {
		t.Errorf("ranked id = %q, want the bindable id without its billing suffix", resp.Models[0].ID)
	}
	// The WINNER's numbers, under the base id: collapsing the lanes must not
	// quietly swap in the lower-scoring row's price or score.
	if resp.Models[0].RankScore == nil || *resp.Models[0].RankScore != "64.2" {
		t.Errorf("ranked score = %v, want the higher-scoring lane's 64.2", resp.Models[0].RankScore)
	}
}
