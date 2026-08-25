// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package people

import "testing"

// The nine companies a CSV import refused to create, against a live database.
//
// The unit test asks the scorer directly. This asks the LADDER — the trigram
// candidate query, the visibility rules and the decision — exactly as an import
// does. Each incumbent below is the real company the matcher named when it told
// an operator that a company of that name was already in the CRM.
func TestTheNineRefusedCompaniesNoLongerCollide(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()

	incumbents := []string{
		"Salesforce", "Soda Capital", "Centric Software", "HealthCare Logic",
		"The Performance Network Group (TPNG)", "Thinksmart Group",
		"MTA TECH", "Elephant Digital", "JTL-Software",
	}
	for _, name := range incumbents {
		if _, err := e.store.CreateOrganization(ctx, CreateOrganizationInput{
			DisplayName: name, Source: "manual",
		}); err != nil {
			t.Fatalf("seeding %q: %v", name, err)
		}
	}

	refused := []string{
		"Bytesforce", "ACY Capital", "Fenwick Software", "TLC Healthcare",
		"The NMG Group", "The Alta Group", "Multimedia Technology",
		"Digital Matter", "Pronto Software",
	}
	for _, name := range refused {
		m := e.dedupeOrgInTx(ctx, t, OrganizationCandidate{DisplayName: name})
		if m.Decision != DecisionNoMatch {
			t.Errorf("%q collided (decision %s, confidence %.4f) — no company of "+
				"that name is in the estate, and the operator was sent looking for one",
				name, m.Decision, m.Confidence)
		}
	}

	// And the control: a real duplicate of a seeded name still meets, so the
	// gate has not simply turned the fuzzy tier off.
	dup := e.dedupeOrgInTx(ctx, t, OrganizationCandidate{
		DisplayName: "Centric Software Pty Ltd",
	})
	if dup.Decision != DecisionFuzzyReview {
		t.Fatalf("a genuine duplicate scored %s (confidence %.4f), want fuzzy_review",
			dup.Decision, dup.Confidence)
	}
}
