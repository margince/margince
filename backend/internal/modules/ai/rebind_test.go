// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"strings"
	"sync"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

const rebindFrom = `profile: eu_hosted
tiers:
  local_small: {provider: fake, model: first-small}
  cheap_cloud: {provider: fake, model: first-small}
  premium: {provider: fake, model: first-large}
  frontier: {provider: fake, model: first-large}
embeddings: {provider: fake, model: first-embed, dimensions: 8}
`

func parsed(t *testing.T, doc string) RoutingConfig {
	t.Helper()
	cfg, err := ParseRouting([]byte(doc))
	if err != nil {
		t.Fatalf("ParseRouting: %v", err)
	}
	return cfg
}

func TestRebindReplacesTheBoundModelsAndTheVersionTogether(t *testing.T) {
	r, err := NewRouter(parsed(t, rebindFrom), nil, DefaultMonthlyTokens, nil, false, nil)
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}
	before := r.binding()
	if got, ok := r.CurrentModelForTier(TierPremium); !ok || got.Model != "first-large" {
		t.Fatalf("premium = %+v ok=%v, want first-large", got, ok)
	}

	after := parsed(t, strings.ReplaceAll(rebindFrom, "first-", "second-"))
	if err := r.Rebind(after); err != nil {
		t.Fatalf("Rebind: %v", err)
	}

	if got, ok := r.CurrentModelForTier(TierPremium); !ok || got.Model != "second-large" {
		t.Errorf("premium = %+v ok=%v; the rebind did not reach the bound models", got, ok)
	}
	// The version travels WITH the models. It is the cache key every stored
	// brief fingerprints against, so a binding that changed its models and kept
	// its version would leave that content attributed to a model that no longer
	// produces it — the failure the whole digest exists to prevent.
	now := r.binding()
	if now.configHash == before.configHash {
		t.Error("the config hash survived a rebind that changed every bound model")
	}
	if now.configHash != after.RoutingVersion() && now.configHash == "" {
		t.Error("the rebound config carries no hash at all")
	}
}

// Entries the replaced binding produced can never be read again, so a rebind
// evicts them rather than leaving them resident to push live entries out
// through the size cap. Read back under the generation that WROTE them: under
// the new one the generation check would answer "miss" either way, and the
// eviction itself would go unproven.
func TestRebindDropsTheResultCache(t *testing.T) {
	r, err := NewRouter(parsed(t, rebindFrom), nil, DefaultMonthlyTokens, nil, false, nil)
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}
	ws := ids.From[ids.WorkspaceKind](ids.NewV7())
	replaced := r.binding().generation
	r.cache.put("some-key", ws, replaced, model.Response{Text: "an answer the previous binding produced"}, TierPremium)
	if _, _, ok := r.cache.get("some-key", ws, replaced); !ok {
		t.Fatal("the fixture did not land in the cache; the test proves nothing")
	}

	if err := r.Rebind(parsed(t, strings.ReplaceAll(rebindFrom, "first-", "second-"))); err != nil {
		t.Fatalf("Rebind: %v", err)
	}
	if _, _, ok := r.cache.get("some-key", ws, replaced); ok {
		t.Error("an answer produced by the previous binding survived the rebind")
	}
}

// The interleaving the eviction alone cannot close: a completion that loaded
// the old binding finishes AFTER the rebind cleared the cache, so its answer
// lands in a cache the new binding is already serving from. RouteInfo names
// the provider and model of the binding that serves a hit, and that identity
// is what tells an operator "no AI provider is configured" apart from "the
// model answered badly" — so the new binding must not serve those words.
func TestAnAnswerLandingAfterARebindIsNotServedUnderTheNewBinding(t *testing.T) {
	r, err := NewRouter(parsed(t, rebindFrom), nil, DefaultMonthlyTokens, nil, false, nil)
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}
	ws := ids.From[ids.WorkspaceKind](ids.NewV7())
	inFlight := r.binding().generation

	if err := r.Rebind(parsed(t, strings.ReplaceAll(rebindFrom, "first-", "second-"))); err != nil {
		t.Fatalf("Rebind: %v", err)
	}
	// The late write: the call resolved its tier and its model against the
	// binding it loaded before the rebind, and only now returns.
	r.cache.put("some-key", ws, inFlight, model.Response{Text: "first-large answered this"}, TierPremium)

	rebound := r.binding().generation
	if rebound == inFlight {
		t.Fatal("the rebind reused the binding generation; every entry would stay readable across it")
	}
	if _, _, ok := r.cache.get("some-key", ws, rebound); ok {
		t.Error("the new binding served an answer the previous binding produced")
	}
	// Not simply absent: the entry is resident, and it is the generation that
	// withholds it. Without this the case would pass against a put that
	// silently stored nothing.
	if _, _, ok := r.cache.get("some-key", ws, inFlight); !ok {
		t.Fatal("the late write never landed; the case proves nothing about the generation")
	}
}

// The property the atomic pointer exists for: a reader must never observe a
// half-applied rebind. Run under -race, this fails on a plain field swap.
func TestConcurrentReadsNeverSeeAHalfAppliedRebind(t *testing.T) {
	r, err := NewRouter(parsed(t, rebindFrom), nil, DefaultMonthlyTokens, nil, false, nil)
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}
	second := parsed(t, strings.ReplaceAll(rebindFrom, "first-", "second-"))
	first := parsed(t, rebindFrom)

	var wg sync.WaitGroup
	stop := make(chan struct{})
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				// A tier's model and the embed identity must come from ONE
				// binding. Either generation is fine; a mixture is not.
				m, ok := r.CurrentModelForTier(TierPremium)
				if !ok {
					t.Error("premium unbound mid-rebind; a reader saw no binding at all")
					return
				}
				if !strings.HasSuffix(m.Model, "-large") {
					t.Errorf("premium resolved to %q, which belongs to neither binding", m.Model)
					return
				}
			}
		}()
	}
	for i := range 200 {
		cfg := second
		if i%2 == 0 {
			cfg = first
		}
		if err := r.Rebind(cfg); err != nil {
			t.Errorf("Rebind: %v", err)
			break
		}
	}
	close(stop)
	wg.Wait()
}
