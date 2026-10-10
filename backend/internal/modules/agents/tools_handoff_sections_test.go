// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// A list the caller may not read is not an empty list. The answer names it and
// withholds the absence claim, as it does for a list cut at its bound.
func TestAListTheCallerMayNotReadIsNamedAndSupportsNoAbsenceClaim(t *testing.T) {
	for _, tc := range []struct {
		name    string
		section crmcontracts.Project360Section
		omit    func(*HandoffFacts)
		gap     string
	}{
		{"deals", sectionDeals, func(f *HandoffFacts) { f.Deals = nil }, gapNoWonDeal},
		{"stakeholders", sectionStakeholders, func(f *HandoffFacts) { f.Stakeholders = nil }, gapNoStakeholder},
	} {
		facts := wholeHandoff()
		tc.omit(&facts)
		if _, raised := gapNamed(prepared(t, facts), tc.gap); !raised {
			t.Fatalf("%s: an empty list the caller MAY read raised no %q, so the withholding below proves nothing", tc.name, tc.gap)
		}
		facts.SectionsOmitted = []crmcontracts.Project360Section{tc.section}
		out := prepared(t, facts)
		if _, raised := gapNamed(out, tc.gap); raised {
			t.Errorf("%s: %q was raised over a list the caller may not read", tc.name, tc.gap)
		}
		if len(out.SectionsOmitted) != 1 || out.SectionsOmitted[0] != tc.section {
			t.Errorf("%s: sections_omitted = %v, want [%s]", tc.name, out.SectionsOmitted, tc.section)
		}
	}
}
