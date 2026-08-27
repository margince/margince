// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package provider_test

// What a provider gives away, and what it only appears to.
//
// The free set decides what automatic enrichment may buy on nobody's say-so,
// so a category wrongly counted free is a spend behind a switch that promises
// none.

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/provider"
)

func TestFreeExcludesACategoryWhoseCascadeCharges(t *testing.T) {
	desc := provider.Descriptor{
		Categories: []provider.Category{"linkedin_profile", "professional_email", "personal_email"},
		CostTable: map[provider.Category]map[provider.Pool]int{
			"linkedin_profile":   {},
			"professional_email": {"email": 1},
			// Free to REQUEST, and that is the trap: asking for it costs
			// nothing until the fallback fires.
			"personal_email": {},
		},
		Cascades: []provider.Cascade{{
			Category: "personal_email",
			After:    "professional_email",
			Cost:     map[provider.Pool]int{"email": 2},
		}},
	}

	free := desc.Free()

	// Only the genuinely free one. Advertising personal_email as free would put
	// a two-credit spend behind a switch labelled "costs nothing".
	if len(free) != 1 || free[0] != "linkedin_profile" {
		t.Errorf("Free() = %v, want only linkedin_profile", free)
	}
}

func TestFreeKeepsTheDescriptorsOwnOrder(t *testing.T) {
	desc := provider.Descriptor{
		Categories: []provider.Category{"zebra", "apple", "mango"},
		CostTable: map[provider.Category]map[provider.Pool]int{
			"zebra": {}, "apple": {}, "mango": {},
		},
	}

	free := desc.Free()

	// The declared order, not a map's. A settings card listing the free
	// categories must not reshuffle them between two reads.
	want := []provider.Category{"zebra", "apple", "mango"}
	for i, category := range want {
		if free[i] != category {
			t.Errorf("position %d is %q, want %q", i, free[i], category)
		}
	}
}
