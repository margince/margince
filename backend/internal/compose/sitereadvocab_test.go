// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"regexp"
	"slices"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
)

// The offering and market guidance is the only thing that stops a customer
// story's facts being filed under this company instead of the customer it is
// about — no code path enforces it, only the model reading the prompt. The
// aicert corpus pins the model's resulting outcome on a live run; this pins
// the sentences themselves, through the same menuGuidance seam the real
// prompt assembles from (menuGuidance has its own history of silently
// dropping a whole category from its hardcoded list, per its own comment),
// so both the guard text AND its routing into the prompt are held.
func TestMenuGuidanceKeepsTheCustomerStoryGuardOutOfThisCompanysOwnFacts(t *testing.T) {
	tests := map[string]struct {
		fields      []string
		mustContain []string
	}{
		"offering": {
			fields: []string{contacts.FactService},
			mustContain: []string{
				"case study, testimonial or customer story",
				"NAMED CUSTOMER, not this company",
				"never this company's offering",
				"is still this company's offering",
			},
		},
		"market": {
			fields: []string{contacts.FactServedIndustry},
			mustContain: []string{
				"case study, testimonial or customer story names a customer's own",
				"never a market this company serves",
			},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			guidance := menuGuidance(tt.fields)
			for _, want := range tt.mustContain {
				if !strings.Contains(guidance, want) {
					t.Errorf("menuGuidance(%v) lost its customer-story guard: missing %q", tt.fields, want)
				}
			}
		})
	}
}

// A field defined but refused by the schema gets its facts filed under another field.
// The kinds are written out because the contract generates no list of them.
func TestMenuGuidanceNamesOnlyTheFieldsOnTheMenu(t *testing.T) {
	kinds := []crmcontracts.SiteReadPageKind{
		crmcontracts.SiteReadPageKindAbout, crmcontracts.SiteReadPageKindContact, crmcontracts.SiteReadPageKindHome,
		crmcontracts.SiteReadPageKindImpressum, crmcontracts.SiteReadPageKindOther, crmcontracts.SiteReadPageKindProducts,
		crmcontracts.SiteReadPageKindServices, crmcontracts.SiteReadPageKindTeam,
	}
	checked := 0
	for _, kind := range kinds {
		menu, ok := menuForKind(kind)
		if !ok || len(menu.factFields) == 0 {
			continue
		}
		checked++
		guidance := menuGuidance(menu.factFields)
		for _, part := range factGuidance {
			for _, field := range part.fields {
				if !slices.Contains(menu.factFields, field) && namesField(guidance, field) {
					t.Errorf("the %s page's guidance names %s, which its menu does not offer", kind, field)
				}
			}
		}
	}
	if checked == 0 {
		t.Fatal("no page kind offers a fact menu, so nothing was checked")
	}
}

func TestMenuGuidanceDefinesEveryFieldTheMenuOffers(t *testing.T) {
	home, ok := menuForKind(crmcontracts.SiteReadPageKindHome)
	if !ok {
		t.Fatal("the home page offers no menu")
	}
	guidance := menuGuidance(home.factFields)
	for _, part := range factGuidance {
		if !strings.Contains(guidance, part.text) {
			t.Errorf("the home page offers %v and its guidance leaves out their definition", part.fields)
		}
	}
}

// namesField reports whether text holds field as a whole word, so "partners"
// does not name the partner field.
func namesField(text, field string) bool {
	return regexp.MustCompile(`(^|[^A-Za-z0-9_])` + regexp.QuoteMeta(field) + `($|[^A-Za-z0-9_])`).MatchString(text)
}
