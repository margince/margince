// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"strings"
	"testing"
)

// categoryGuidance's "offering" and "market" prompt text is the only defense
// against a case study, testimonial or customer story getting attributed to
// this company instead of the customer it is about (margince#3403) — no
// deterministic code path enforces it, only the model reading the prompt. The
// aicert corpus (site_fact_extract/customer_story_01.yaml) pins the resulting
// MODEL OUTCOME, but nothing pinned the guard TEXT itself: deleting these
// sentences would fail no Go test, only degrade a probabilistic cert score
// over time. This test is that missing deterministic layer.
func TestCategoryGuidanceGuardsAgainstAttributingACustomersStoryToThisCompany(t *testing.T) {
	tests := map[string]struct {
		category    string
		mustContain []string
	}{
		"offering": {
			category: "offering",
			mustContain: []string{
				"case study, testimonial or customer story",
				"NAMED CUSTOMER, not this company",
				"never this company's offering",
			},
		},
		"market": {
			category: "market",
			mustContain: []string{
				"case study, testimonial or customer story names a customer's own",
				"never a market this company serves",
			},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			guidance, ok := categoryGuidance[tt.category]
			if !ok {
				t.Fatalf("categoryGuidance has no entry for %q", tt.category)
			}
			for _, want := range tt.mustContain {
				if !strings.Contains(guidance, want) {
					t.Errorf("categoryGuidance[%q] lost its customer-story guard: missing %q", tt.category, want)
				}
			}
		})
	}
}
