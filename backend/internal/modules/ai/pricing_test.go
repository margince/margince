// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

func TestPriceCall(t *testing.T) {
	cases := []struct {
		name string
		u    Usage
		r    ModelRate
		want int64
	}{
		// Anthropic-shaped: 700 in total (100 uncached + 400 read + 200 write), 50 out
		// rates: in $5/MTok=5_000_000, out $25/MTok=25_000_000, read 500_000, write 6_250_000
		// = (100×5e6 + 400×5e5 + 200×6.25e6 + 50×25e6)/1e6 = (5e8+2e8+1.25e9+1.25e9)/1e6 = 3200
		{
			"anthropic cache-heavy",
			Usage{TokensIn: 700, CachedTokens: 400, CacheWriteTokens: 200, TokensOut: 50},
			ModelRate{InputPerMTokMicroUSD: 5_000_000, OutputPerMTokMicroUSD: 25_000_000, CacheReadPerMTokMicroUSD: 500_000, CacheWritePerMTokMicroUSD: 6_250_000},
			3200,
		},
		// plain call, zero-rate local → 0
		{
			"local zero rate",
			Usage{TokensIn: 1000, TokensOut: 1000},
			ModelRate{},
			0,
		},
		// a row publishing no cache price: 300 uncached + 400 read + 200 write
		// all at the $1.50 input rate, 100 out at $7.50
		// = (900×1.5e6 + 100×7.5e6)/1e6 = (1.35e9+7.5e8)/1e6 = 2100
		{
			"zero cache price bills the input rate",
			Usage{TokensIn: 900, CachedTokens: 400, CacheWriteTokens: 200, TokensOut: 100},
			ModelRate{InputPerMTokMicroUSD: 1_500_000, OutputPerMTokMicroUSD: 7_500_000},
			2100,
		},
		// only the write price is unpublished: 400 read at $0.15, the rest at $1.50
		// = (300×1.5e6 + 400×1.5e5 + 200×1.5e6)/1e6 = (4.5e8+6e7+3e8)/1e6 = 810
		{
			"zero cache-write price bills the input rate, cache read keeps its own",
			Usage{TokensIn: 900, CachedTokens: 400, CacheWriteTokens: 200},
			ModelRate{InputPerMTokMicroUSD: 1_500_000, CacheReadPerMTokMicroUSD: 150_000},
			810,
		},
		// floor: cached > tokens_in leaves the uncached bucket at 0, not -40;
		// the 50 cached price at their own rate = 50×5e5/1e6 = 25
		{
			"defensive floor",
			Usage{TokensIn: 10, CachedTokens: 50},
			ModelRate{InputPerMTokMicroUSD: 5_000_000, CacheReadPerMTokMicroUSD: 500_000},
			25,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := PriceCall(c.u, c.r); got != c.want {
				t.Fatalf("PriceCall(%+v, %+v) = %d, want %d", c.u, c.r, got, c.want)
			}
		})
	}
}

// TestSeedModelRatesEveryEntryIsNonNegativeAndUnique proves the seed set is
// a valid price sheet on its own terms — the fitness the brief asks for
// instead of hand-listing every model: no entry ever pays a negative
// price, and (provider, model) never collides (a duplicate would make
// SeedModelRates' insertion order silently decide which price wins).
// seedRatesTestDay pins the effective date these tests hand SeedModelRates.
// None of them asserts on that date, so a real clock would only give them a
// way to differ between runs.
var seedRatesTestDay = time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC)

func TestSeedModelRatesEveryEntryIsNonNegativeAndUnique(t *testing.T) {
	rates := SeedModelRates(time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC))
	if len(rates) == 0 {
		t.Fatal("SeedModelRates returned no rows")
	}
	seen := make(map[string]bool, len(rates))
	for _, r := range rates {
		if r.InputPerMTokMicroUSD < 0 || r.OutputPerMTokMicroUSD < 0 ||
			r.CacheReadPerMTokMicroUSD < 0 || r.CacheWritePerMTokMicroUSD < 0 {
			t.Errorf("%s/%s: negative rate %+v", r.Provider, r.ModelID, r)
		}
		key := r.Provider + "\x00" + r.ModelID
		if seen[key] {
			t.Errorf("duplicate (provider, model) %s/%s", r.Provider, r.ModelID)
		}
		seen[key] = true
		if r.EffectiveDate.IsZero() {
			t.Errorf("%s/%s: zero EffectiveDate", r.Provider, r.ModelID)
		}
	}
}

// TestSeedModelRatesLocalsAreZero proves every local/offline provider's
// seed row prices as an honest 0 — a local deployment must never read as
// "unpriced" for lack of a rate row (global constraint: price-on-read,
// no silent 0 for a REAL call, but locals are a real 0 by construction).
func TestSeedModelRatesLocalsAreZero(t *testing.T) {
	rates := SeedModelRates(seedRatesTestDay)
	locals := map[string]bool{ProviderFake: false, providerOllama: false, providerVLLM: false, providerLaya: false}
	for _, r := range rates {
		if _, ok := locals[r.Provider]; !ok {
			continue
		}
		locals[r.Provider] = true
		if r.InputPerMTokMicroUSD != 0 || r.OutputPerMTokMicroUSD != 0 ||
			r.CacheReadPerMTokMicroUSD != 0 || r.CacheWritePerMTokMicroUSD != 0 {
			t.Errorf("local provider %s/%s carries a non-zero rate: %+v", r.Provider, r.ModelID, r)
		}
	}
	for provider, present := range locals {
		if !present {
			t.Errorf("no seed row for local provider %q", provider)
		}
	}
}

// TestSeedModelRatesPricesEveryBindingTheShippedSeedsName derives the
// obligation from the tree rather than from a list somebody remembers to
// update: a call whose (provider, model) has no rate row reports UNPRICED,
// which is a materially different signal from FREE, and a binding this repo
// ships for an operator to boot on must not produce one.
//
// The corpus moved with the artefact. It used to be the example ROUTING files;
// those are retired, and what a fresh installation now boots bound to is
// `seeds.ai_routing` in the shipped margince yamls. Same obligation, read off
// whatever the tree actually ships — a vanilla config that deliberately binds
// nothing contributes nothing and is not a failure.
// deliberatelyUnpriced are bindings the seeds leave UNPRICED on purpose.
//
// The one legitimate reason: ModelRate's four billed buckets cannot express the
// model's actual charges, so any single row would be a confident wrong number.
// pricing.go states the invariant that makes this the better answer — "a call
// with no rate row is UNPRICED, which is a materially different signal from
// FREE" — and DayCost.UnpricedCalls surfaces it in the report, where a 2x-wrong
// price would surface as a price.
//
// Ratified rather than tolerated, because the two failures look identical from
// here: a binding nobody priced and a binding somebody could not price differ
// only in whether anyone decided.
var deliberatelyUnpriced = gatekit.Waive(map[string]string{
	"gemini/gemini-3.1-pro-preview": "the only Gemini model whose rate varies with prompt size " +
		"($2/$12 per MTok to 200k, $4/$18 above it), and ModelRate has no context-length break — " +
		"so one row must be ~2x wrong in one direction or the other. Explicit caching is a second " +
		"charge it cannot express: billed per hour HELD rather than per token processed. Nothing " +
		"is billed on it today because no ladder selects the frontier tier. #1045",
})

func TestSeedModelRatesPricesEveryBindingTheShippedSeedsName(t *testing.T) {
	priced := map[string]bool{}
	for _, r := range SeedModelRates(seedRatesTestDay) {
		priced[r.Provider+"/"+r.ModelID] = true
	}
	paths, err := filepath.Glob("../../../../config/margince*.yaml")
	if err != nil {
		t.Fatalf("globbing the shipped configs: %v", err)
	}
	// NOT a tolerated zero: the tree ships margince yamls, so an empty glob
	// means the path moved and this gate went blind while still reporting PASS.
	if len(paths) == 0 {
		t.Fatal("no config/margince*.yaml found — the corpus moved and this gate is checking nothing")
	}
	bound := 0
	for _, path := range paths {
		cfg, ok := seedRoutingIn(t, path)
		if !ok {
			continue // a config that binds nothing owes nothing
		}
		bound++
		// The embeddings lane is not a Tier, so the two are walked as
		// labelled bindings rather than forced into one map.
		bindings := map[string]ProviderConfig{"embeddings": cfg.Embeddings.ProviderConfig}
		for tier, binding := range cfg.Tiers {
			bindings[string(tier)] = binding
		}
		for lane, binding := range bindings {
			model := binding.Provider + "/" + binding.Model
			if priced[model] || deliberatelyUnpriced.Waived(t, model) {
				continue
			}
			t.Errorf("%s binds %s to %s, which SeedModelRates does not price — every call on it "+
				"would report UNPRICED.\n"+
				"  Seed a rate, or — if the model's charges cannot be expressed by ModelRate's four "+
				"buckets — record it in deliberatelyUnpriced with the reason and the issue.\n"+
				"  An unpriced binding nobody declared is indistinguishable from one somebody forgot.",
				filepath.Base(path), lane, model)
		}
	}
	// The dev config binds a real ladder, so a run where NOTHING bound means the
	// seeds block moved or stopped parsing — and this gate would report PASS
	// having read no binding at all.
	// A declared exception for a binding no config names any more is a reason
	// nobody is paying: the model moved on and the note outlived it.
	deliberatelyUnpriced.AssertAllMatched(t)
	if bound == 0 {
		t.Fatal("no shipped config declared seeds.ai_routing — the gate read no binding and would pass regardless")
	}
}

// seedRoutingIn reads seeds.ai_routing out of a shipped margince yaml and parses
// it as the binding it is. ok=false for a config that declares none, which is
// the vanilla posture and not a failure.
func seedRoutingIn(t *testing.T, path string) (RoutingConfig, bool) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	var doc struct {
		Seeds struct {
			AIRouting yaml.Node `yaml:"ai_routing"`
		} `yaml:"seeds"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("%s is not parseable yaml: %v", path, err)
	}
	if doc.Seeds.AIRouting.IsZero() {
		return RoutingConfig{}, false
	}
	inner, err := yaml.Marshal(&doc.Seeds.AIRouting)
	if err != nil {
		t.Fatalf("%s: re-encoding seeds.ai_routing: %v", path, err)
	}
	cfg, err := ParseRouting(inner)
	if err != nil {
		t.Fatalf("%s: seeds.ai_routing no longer parses as a binding: %v", path, err)
	}
	return cfg, true
}

// TestSeedModelRatesEveryRowDeclaresItsLane proves the seed sheet says which
// LANE each model serves, which is what makes it a catalogue and not only a
// price list: the routing form offers a chat model where a chat tier binds and
// an embedder where the embeddings lane binds, and it has nothing but this to
// tell them apart. Deriving the lane from a zero output price would be a guess
// that a chat model priced at 0 (every local row) gets wrong.
func TestSeedModelRatesEveryRowDeclaresItsLane(t *testing.T) {
	for _, r := range SeedModelRates(seedRatesTestDay) {
		if r.Lane != LaneChat && r.Lane != LaneEmbeddings && r.Lane != LaneDecisions {
			t.Errorf("%s/%s: lane %q is none of %q, %q, %q", r.Provider, r.ModelID, r.Lane, LaneChat, LaneEmbeddings, LaneDecisions)
		}
	}
}

// TestSeedModelRatesFilesTheEmbeddersAsEmbedders names the embedding models
// outright. The sibling test above proves every row carries A lane; this one
// proves the lane is the RIGHT one, which no property of the row can show —
// an embedder mis-filed as chat is offered on four tier pickers where it
// cannot serve a single call, and is missing from the one lane that needs it.
func TestSeedModelRatesFilesTheEmbeddersAsEmbedders(t *testing.T) {
	embedders := map[string]bool{
		providerGemini + "/gemini-embedding-001":                    false,
		providerOllama + "/bge-m3":                                  false,
		providerOpenAICompatible + "/mistralai/mistral-embed-2312":  false,
		providerOpenAICompatible + "/baai/bge-m3":                   false,
		providerOpenAICompatible + "/openai/text-embedding-3-small": false,
	}
	for _, r := range SeedModelRates(seedRatesTestDay) {
		key := r.Provider + "/" + r.ModelID
		if _, known := embedders[key]; known {
			embedders[key] = true
			if r.Lane != LaneEmbeddings {
				t.Errorf("%s is an embedding model, filed as lane %q", key, r.Lane)
			}
			continue
		}
		if r.Lane == LaneEmbeddings {
			t.Errorf("%s is filed as an embedder and is not one of the known embedding models — "+
				"add it to this list if it is, or correct its lane", key)
		}
	}
	for key, seen := range embedders {
		if !seen {
			t.Errorf("no seed row for embedding model %s — the embeddings picker lost an option", key)
		}
	}
}

// A decision model is filed under the decisions lane and a decision provider
// has no other kind of row, so the routing form offers it only where the
// decisions lane binds. Jev bills its input alone, at the rate its own
// usage.cost showed.
func TestTheSeedSheetFilesJevUnderTheDecisionsLane(t *testing.T) {
	jevPriced := false
	for _, r := range SeedModelRates(seedRatesTestDay) {
		d, _ := providerByName(r.Provider)
		if d.caps.has(capDecision) != (r.Lane == LaneDecisions) {
			t.Errorf("%s/%s: lane %q on a provider whose decision capability is %v", r.Provider, r.ModelID, r.Lane, d.caps.has(capDecision))
		}
		if r.Lane == LaneDecisions && (r.OutputPerMTokMicroUSD != 0 || r.CacheReadPerMTokMicroUSD != 0 || r.CacheWritePerMTokMicroUSD != 0) {
			t.Errorf("%s/%s: a decision row prices a bucket the lane never records", r.Provider, r.ModelID)
		}
		if r.Provider == providerOpenRouterDecision && r.ModelID == "typesafe/jev-1.13" {
			jevPriced = r.InputPerMTokMicroUSD == 42_000
		}
	}
	if !jevPriced {
		t.Error("no seed row prices typesafe/jev-1.13 at 42,000 micro-USD per million input tokens")
	}
}
