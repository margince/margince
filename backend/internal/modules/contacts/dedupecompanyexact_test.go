// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import "testing"

// The bar for "this is the same name" is strict: case, accents and spacing are
// noise, and a legal suffix is not.
func TestOneNameIsTheSameNameOnlyWhenItIsTheSameName(t *testing.T) {
	for _, tc := range []struct {
		name        string
		left, right string
		want        bool
	}{
		{"the same name", "Baqend GmbH", "Baqend GmbH", true},
		{"shouted", "Baqend GmbH", "BAQEND GMBH", true},
		{"spaced by its markup", "Baqend  GmbH", "Baqend GmbH", true},
		{"padded", "  Baqend GmbH  ", "Baqend GmbH", true},
		{"accented", "Zürich Versicherung", "Zurich Versicherung", true},
		// The distinction the whole feature turns on: these two fold together
		// under a suffix-stripping key, and they are two legal entities.
		{"a different legal form", "Baqend GmbH", "Baqend Inc", false},
		{"the bare name against the full one", "Baqend", "Baqend GmbH", false},
		{"a different company", "Baqend GmbH", "Speedkit GmbH", false},
		{"nothing at all", "", "", false},
		{"nothing against something", "", "Baqend GmbH", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := companyNamesAreTheSame(tc.left, tc.right); got != tc.want {
				t.Fatalf("%q vs %q = %v, want %v", tc.left, tc.right, got, tc.want)
			}
		})
	}
}

// Exactness is asked of every eligible pairing, not read off the winner.
//
// The winner is chosen on confidence alone and replaced only by a STRICTLY
// greater score, so a pairing that ties the exact one keeps the top slot. If
// exactness travelled with the winner, the tie would hide it — and the domain
// would mint a second company for a name the workspace already holds.
func TestAnExactNameSurvivesATieOnTheScore(t *testing.T) {
	// Both companies trade under one name and are registered under the other.
	// Four pairings are eligible and all score 1.0, so the FIRST one examined
	// keeps the winner's slot — and that one is "Baqend Inc" against "Baqend
	// GmbH", which is not the same name. The exact pairing is the later
	// display-against-legal one, and it is a tie, so it never displaces the
	// winner.
	//
	// Reading exactness off the winner therefore reports false here, and the
	// domain mints a second company for a name the workspace already holds.
	best := bestCompanyNamePairing("Baqend Inc", "Baqend GmbH", "Baqend GmbH", "Baqend Inc")
	if companyNamesAreTheSame(best.CandidateValue, best.IncumbentValue) {
		t.Fatalf("the winning pairing %q vs %q is itself exact, so this case no longer "+
			"exercises a tie the winner loses — pick names where it does",
			best.CandidateValue, best.IncumbentValue)
	}
	if !best.ExactName {
		t.Fatal("an exact pairing existed and was not reported, because the winner was " +
			"chosen on score alone: a name twin would create a second company")
	}

	near := bestCompanyNamePairing("Baqend Inc", "", "Baqend GmbH", "")
	if near.ExactName {
		t.Fatalf("%q and %q were marked the same name; they are two legal entities and a human's call",
			near.CandidateValue, near.IncumbentValue)
	}
	if near.Confidence == 0 {
		t.Fatal("the near-match scored nothing, so this case is not exercising the fuzzy tier it claims to")
	}
}
