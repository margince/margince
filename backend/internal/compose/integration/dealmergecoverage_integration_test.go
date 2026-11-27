// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The deal merge's coverage census. Every foreign key pointing at deal is a
// row the merge has to decide about. The database supplies the list, so a table
// that gains a deal_id is enrolled by its migration. The rule is the company
// census's, in companymergecoverage_integration_test.go. This file holds the
// deal's own paths, floor and waivers.

import (
	"io/fs"
	"path/filepath"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
	"github.com/margince/margince/backend/pkg/extension/reach"
)

// theDealFKFloor is the count below which the census has broken rather than the
// schema having shrunk. 30 columns point at deal today.
const theDealFKFloor = 28

// dealMergePathFiles are the files that make up the deal merge's relink: its
// own statements and the shared steps it calls.
var dealMergePathFiles = []string{
	"internal/modules/deals/dealmerge.go",
	"internal/modules/deals/dealmergerelink.go",
	"internal/platform/recordmerge/links.go",
	"internal/platform/recordmerge/pair.go",
	"internal/platform/recordmerge/kindid.go",
	"internal/modules/identity/grantmerge.go",
}

// dealMergeSeamFiles are the owning modules' seams, found by name so a module
// that gains one is read without anyone adding it to a list. Finding none
// would pass every table as unwritten, so the census refuses an empty answer.
func dealMergeSeamFiles(t *testing.T) []string {
	t.Helper()
	var found []string
	err := filepath.WalkDir("../../../internal", func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && d.Name() == "mergedealrefs.go" {
			rel, relErr := filepath.Rel("../../..", path)
			found = append(found, filepath.ToSlash(rel))
			return relErr
		}
		return err
	})
	if err != nil || len(found) < 8 {
		t.Fatalf("found %d owner seams (%v), want the eight modules that keep rows about a deal", len(found), err)
	}
	return found
}

// dealFKsTheMergeLeaves are the deal-pointing columns the merge does not move.
// A deal's own path stays with the deal that took it. The stage-age report
// counts from the latest entry into the current stage, so a copied history
// would make the survivor look younger.
var dealFKsTheMergeLeaves = gatekit.Waive(map[string]string{
	"deal_stage_history.deal_id":            "the path this deal took through its stages; the survivor did not take it, and the stage-age report would read a copied history as a younger deal",
	"deal_stage_evidence.deal_id":           "what supported the retired deal's stage moves; it belongs to the history above and goes with it",
	"deal_correction.deal_id":               "the corrections made to the retired deal's own stage and close dates, tied to its history rows",
	"close_date_run_member.deal_id":         "a close-date sweep run's record of what it proposed for the retired deal at the time",
	"activity_review_response.deal_id":      "the closing review answered against one of the retired deal's closing occurrences; its composite foreign key to deal_stage_history cannot name another deal",
	"deal_risk_verdict.deal_id":             "a verdict derived from the retired deal's own state on one day; the survivor gets its own on the next pass",
	"generated_summary.deal_id":             "the retired deal's cached status card; the survivor's card is derived from the survivor",
	"deal_move_dismissal.deal_id":           "a reader's set-aside of a suggested move on the retired deal, keyed by a fingerprint over that deal's own state",
	"assurance_task_item.subject_deal_id":   "an open finding on the retired deal closes as subject_departed once the deal is archived; the survivor is re-examined by the next scan",
	"deal_suggestion.accepted_deal_id":      "the frozen record of which suggestion created the retired deal",
	"sales_initiative_baseline.deal_id":     "the figure an initiative counted for the retired deal when its baseline was taken; moving it would change a number already reported",
	"deal_line_item.deal_id":                "the retired deal's own price composition; lines moved onto a survivor would change its mode and amount without anyone saying so, and its position order is unique per deal",
	"deal_product.deal_id":                  "the retired deal's products, the companion of its lines and left with them for the same reason",
	"deal.merged_into_id":                   "the redirect pointer, set on the retired deal by recordmerge.ArchiveMergedAway, whose statement takes its table as a parameter and so cannot be read as a deal write; TestDealMergeMovesTheFactsAndLeavesThePathOnTheLoser asserts it lands",
	"signal.entity_deal_id":                 "a trigger fills it from entity_type and entity_id, and the shared kind-and-id move carries that pair, so the key follows it to the survivor",
	"weekly_plan_commitment.linked_deal_id": "a trigger fills it from linked_record_type and linked_record_id, and the weekly plan's sweep reach declaration moves that pair, so the key follows it to the survivor",
})

// TestEveryDealForeignKeyJoinsTheMerge fails when a table carrying a deal
// foreign key is neither written by the merge path nor declared above.
func TestEveryDealForeignKeyJoinsTheMerge(t *testing.T) {
	t.Parallel()
	defer dealFKsTheMergeLeaves.AssertAllMatched(t)

	assertEveryForeignKeyJoinsTheMerge(t, mergeCensus{
		table: "deal", floor: theDealFKFloor, leaves: dealFKsTheMergeLeaves,
		written: tablesTheMergeWrites(t, append(slices.Clone(dealMergePathFiles), dealMergeSeamFiles(t)...), reach.Deal),
		moveIn:  "internal/modules/deals/dealmergerelink.go", leavesName: "dealFKsTheMergeLeaves",
	})
}

// TestEveryDealKindIDReferenceJoinsTheMerge is the same census for the rows
// that name a deal by kind and id, which no foreign key shows.
func TestEveryDealKindIDReferenceJoinsTheMerge(t *testing.T) {
	t.Parallel()
	defer dealKindIDPairsTheMergeLeaves.AssertAllMatched(t)

	assertEveryKindIDReferenceJoinsTheMerge(t, reach.Deal, append(slices.Clone(dealMergePathFiles), dealMergeSeamFiles(t)...), dealKindIDPairsTheMergeLeaves)
}

// dealKindIDPairsTheMergeLeaves are the pairs only a deal can sit in, because
// their closed vocabularies include deal and exclude company. The pairs every
// merge leaves are kindIDPairsTheMergeLeaves.
var dealKindIDPairsTheMergeLeaves = gatekit.Waive(map[string]string{
	"contact_acquisition_evidence.source_entity_id": "records which entity a contact was first seen through, as it was when the contact was acquired; moving it would rewrite where the contact came from",
	"sales_initiative_enrolment.record_id":          "an initiative's enrolment of the retired deal, counted against its baseline when it was taken; a moved row would collide with the survivor's own enrolment and change a figure already reported",
	"sales_initiative_member.record_id":             "the member row of an initiative for the retired deal, the companion of the enrolment above and left with it for the same reason",
	"weekly_review_learning_citation.subject_id":    "a citation of the retired deal in a weekly review's learning, frozen with the review it was written for",
})
