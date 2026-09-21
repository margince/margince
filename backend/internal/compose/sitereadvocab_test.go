// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"strings"
	"testing"

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
