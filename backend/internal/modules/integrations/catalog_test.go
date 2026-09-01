// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package integrations

// The price list a settings card and a buy button both read.
//
// It is derived from the descriptor rather than listed, so the two questions
// it answers — what does this cost, and is it free — cannot disagree with the
// cost table they are computed from.

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/ports/provider"
)

func TestTheCatalogPricesEveryCategoryTheProviderDeclares(t *testing.T) {
	desc := provider.Descriptor{
		Categories: []provider.Category{"linkedin_profile", "professional_email", "mobile"},
		CostTable: map[provider.Category]map[provider.Pool]int{
			"linkedin_profile":   {},
			"professional_email": {"email": 1},
			"mobile":             {"mobile": 1},
		},
	}

	catalog := catalogOf(desc)

	// One entry per declared category. A provider that adds a category and
	// finds it missing here would show a settings card that cannot say what
	// the new thing costs.
	if len(catalog) != len(desc.Categories) {
		t.Fatalf("catalog has %d entries, want one per declared category (%d)",
			len(catalog), len(desc.Categories))
	}
	for i, category := range desc.Categories {
		if catalog[i].Category != string(category) {
			t.Errorf("entry %d is %q, want %q — the descriptor's own order", i, catalog[i].Category, category)
		}
	}
}

func TestACatalogEntryAgreesWithTheCostTableItComesFrom(t *testing.T) {
	desc := provider.Descriptor{
		Categories: []provider.Category{"linkedin_profile", "professional_email"},
		CostTable: map[provider.Category]map[provider.Pool]int{
			"linkedin_profile":   {},
			"professional_email": {"email": 1},
		},
	}

	catalog := catalogOf(desc)

	byName := map[string]CategoryCost{}
	for _, entry := range catalog {
		byName[entry.Category] = entry
	}
	// Free means no pool is charged, and the price is the pools that are. A
	// button quoting a figure the reservation does not take would be asking
	// somebody to agree to the wrong number.
	if !byName["linkedin_profile"].Free || len(byName["linkedin_profile"].Cost) != 0 {
		t.Errorf("linkedin_profile = %+v, want free with no cost", byName["linkedin_profile"])
	}
	if byName["professional_email"].Free {
		t.Error("professional_email reads free, and the cost table charges an email credit for it")
	}
	if byName["professional_email"].Cost["email"] != 1 {
		t.Errorf("professional_email costs %v, want 1 email credit", byName["professional_email"].Cost)
	}
}

func TestAFallbackIsPricedWithTheCategoryThatTriggersIt(t *testing.T) {
	desc := provider.Descriptor{
		Categories: []provider.Category{"professional_email", "personal_email"},
		CostTable: map[provider.Category]map[provider.Pool]int{
			"professional_email": {"email": 1},
			// Free to request on its own, which is the trap.
			"personal_email": {},
		},
		Cascades: []provider.Cascade{{
			Category: "personal_email",
			After:    "professional_email",
			Cost:     map[provider.Pool]int{"email": 2},
		}},
	}

	byName := map[string]CategoryCost{}
	for _, entry := range catalogOf(desc) {
		byName[entry.Category] = entry
	}

	// A cascade bills only when its trigger was requested too, so pricing the
	// fallback alone reads as free — the understatement a buy button must
	// never make about what pressing it can spend.
	fallback := byName["personal_email"]
	if fallback.Free {
		t.Error("personal_email reads free, and issuing it costs two email credits")
	}
	if fallback.Cost["email"] != 3 {
		t.Errorf("personal_email costs %v, want the trigger's 1 plus the fallback's 2", fallback.Cost)
	}
}

func TestAProviderThatGivesNothingAwayStillConnects(t *testing.T) {
	desc := provider.Descriptor{
		Categories:    []provider.Category{"premium_only"},
		DefaultPreset: "full",
		Presets: map[string][]provider.Category{
			"full": {"premium_only"},
		},
		CostTable: map[provider.Category]map[provider.Pool]int{
			"premium_only": {"credits": 5},
		},
	}

	cfg, err := resolveConfig(desc, nil)
	if err != nil {
		t.Fatalf("resolving the default config: %v", err)
	}

	// The column requires at least one category. An empty selection would be
	// refused by the database with a constraint error rather than a sentence
	// anybody can act on, and a connection nobody can make is worse than one
	// whose first run costs a credit.
	if len(cfg.Categories) == 0 {
		t.Error("a provider with no free categories resolved to an empty selection, which the connection column refuses")
	}
}

func TestEveryRegisteredAdapterPricesWhatItDeclares(t *testing.T) {
	// The real adapters this build can register, not a stand-in: a stub would
	// prove the check runs, and this proves the shipped descriptors pass it.
	for _, adapter := range []provider.Adapter{NewOfflineProvider(0, time.Now)} {
		desc := adapter.Descriptor()
		t.Run(desc.Name, func(t *testing.T) {
			for _, category := range desc.Categories {
				if _, priced := desc.CostTable[category]; !priced {
					// A missing entry reads as free everywhere the platform
					// asks what something costs, so it would be bought
					// automatically and reserved at nothing. An empty map says
					// "free" deliberately; an omission says nothing at all.
					t.Errorf("category %q has no cost entry: price it, or declare it free with an empty one", category)
				}
			}
		})
	}
}

// A category the provider only issues alongside another must be priced with
// it. The buy button asks for both (boughtWith, frontend/src/screens/
// personprovider.tsx) and quotes ONE figure — this entry's — so an entry
// carrying only its own price would name a number smaller than the press
// spends.
//
// Derived from the descriptor's own RequiresAnswerTo rather than naming a
// category: the defect is a property of the pairing rule, so an adapter that
// declares a new pair is covered the day it lands.
//
// Run over the offline fake, which is the only adapter this module may import
// — a module never imports a sibling, and surfe is one. That costs nothing
// here: TestTheOfflineFakeDescribesTheSameProductAsTheLiveAdapter
// (backend/internal/compose/providerdescriptorparity_test.go) compares
// RequiresAnswerTo and CostTable between the fake and the live adapter, so a
// fake that passes this and a live adapter that would not cannot both exist.
func TestAPricedCategoryCarriesItsPrerequisitesPrice(t *testing.T) {
	desc := NewOfflineProvider(0, func() time.Time { return time.Unix(0, 0).UTC() }).Descriptor()
	if len(desc.RequiresAnswerTo) == 0 {
		// Under-recognition is the one way this gate must not break: with no
		// pairs to walk it would report PASS over a rule nobody checked.
		t.Fatal("the fake declares no prerequisites, so this gate walked nothing — it mirrors an adapter that has one")
	}
	entries := map[string]CategoryCost{}
	for _, entry := range catalogOf(desc) {
		entries[entry.Category] = entry
	}
	for category, prerequisite := range desc.RequiresAnswerTo {
		paired, ok := entries[string(category)]
		if !ok {
			t.Errorf("category %q has a prerequisite but no catalog entry to price it in", category)
			continue
		}
		if paired.Requires != string(prerequisite) {
			t.Errorf("%q names prerequisite %q, catalog entry says %q",
				category, prerequisite, paired.Requires)
		}
		// Pool by pool, because a pair spanning two pools is the case here:
		// summing to one total would pass a price that charged the mobile
		// pool twice and the email pool not at all.
		for pool, n := range desc.CostTable[prerequisite] {
			if paired.Cost[string(pool)] < n {
				t.Errorf("%q costs %v, which does not cover its prerequisite %q at %d in pool %q — the button would quote less than the press spends",
					category, paired.Cost, prerequisite, n, pool)
			}
		}
	}
}
