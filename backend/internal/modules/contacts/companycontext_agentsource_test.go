// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import "testing"

// A company name an agent wrote reads as the agent's. A site read proposing
// another one is a conflict the user decides, as over their own value.
func TestACompanyValueAnAgentWroteKeepsItsSource(t *testing.T) {
	if got := normalizeCompanySource(CompanySourceAgent); got != CompanySourceAgent {
		t.Fatalf("normalizeCompanySource(agent) = %q, want it kept", got)
	}
	comparison := classifySiteReadValue("display_name", siteReadValueProfile, "From the site",
		"From the agent", normalizeCompanySource(CompanySourceAgent), true)
	if comparison.Classification != siteReadComparisonHumanConflict {
		t.Fatalf("classification = %q, want the conflict a user decides", comparison.Classification)
	}
}
