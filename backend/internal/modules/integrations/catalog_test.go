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
