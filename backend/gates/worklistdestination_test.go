// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// Every source the worklist can emit has one screen it belongs on.
//
// The destination decides where a row is drawn AND which bucket counts it, so a
// source with no answer is a row that reaches a reader on whatever screen the
// zero value picked, under a count that put it somewhere else. Nothing else in
// the tree fails when that happens: the row renders, the figures add up, and
// the only symptom is a duplicate pair sitting in a seller's morning.
//
// BOTH SIDES ARE DERIVED. The vocabulary comes from crm.yaml, the classified
// set from the map's own keys. A list repeated here would be a second copy of
// the subject, and the copy is what goes stale — this gate would then read a
// smaller world, report PASS, and the new source would be exactly as unclassified
// as it was before anybody wrote a gate.

import (
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/compose/attention"
)

// batchStandsForOtherRows names a group rather than a record, so its screen is
// its members' — and a mapping for the word `batch` would be a second answer to
// a question the members already answer.
//
// Held by: TestEveryWorklistSourceHasADestination, which fails in both
// directions — a source the contract declares and the map does not classify,
// and this one if it ever is classified.
const batchStandsForOtherRows = "batch"

func TestEveryWorklistSourceHasADestination(t *testing.T) {
	t.Parallel()
	declared := crmYAMLEnum(t, "WorklistItem", "source")
	classified := []string{}
	for _, source := range attention.ClassifiedSources() {
		classified = append(classified, string(source))
	}
	slices.Sort(classified)

	for _, source := range declared {
		if source == batchStandsForOtherRows {
			if slices.Contains(classified, source) {
				t.Errorf("%q is classified by source, but it stands for a group of other rows: "+
					"its screen is its members', decided where the group is minted", source)
			}
			continue
		}
		if !slices.Contains(classified, source) {
			t.Errorf("the worklist can emit %q and nothing says which screen it belongs on: "+
				"add it to destinationOfSource in internal/compose/attention/destination.go, "+
				"choosing today, review, system_health or receipt", source)
		}
	}
	// And the other direction: a classification for a source the contract no
	// longer declares is a decision about nothing, which reads to the next
	// author as though that source still exists.
	for _, source := range classified {
		if !slices.Contains(declared, source) {
			t.Errorf("destinationOfSource classifies %q, which crm.yaml no longer declares as a worklist source", source)
		}
	}
}

// TestEveryDestinationIsOneTheContractDeclares holds the values themselves.
//
// The map's answers reach the wire, so a destination spelled here that the
// schema does not carry is a response a client cannot parse — and the failure
// would arrive as a validation error on a reader's queue rather than here.
func TestEveryDestinationIsOneTheContractDeclares(t *testing.T) {
	t.Parallel()
	declared := crmYAMLEnum(t, "WorklistItem", "destination")
	for _, source := range attention.ClassifiedSources() {
		at := string(attention.DestinationOfSource(source))
		if !slices.Contains(declared, at) {
			t.Errorf("source %q is sent to %q, which crm.yaml does not declare as a destination", source, at)
		}
	}
}
