// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"encoding/json"
	"testing"

	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/diffhash"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// A card cites only sources a reviewer can open. A claim whose source is
// deleted or hidden still settles its criterion, but its pointer is left off.
func TestAProgressionCardDropsACitationNobodyCanOpen(t *testing.T) {
	kept, gone := ids.NewV7(), ids.NewV7()
	facts := deals.StageProgressionFacts{Criteria: []deals.ProgressionCriterion{{
		Label: "Buyer confirmed the problem",
		Sources: []deals.EvidenceSource{
			{SourceType: "activity", SourceID: kept, Snippet: "Yes, that is our problem."},
			{SourceType: "activity", SourceID: gone, Snippet: "We agree.", Missing: true},
		},
	}}}
	facts.Criteria[0].Met = true

	got := progressionEvidence(facts)
	if len(got) != 1 || got[0].SourceID != kept {
		t.Fatalf("the card cites %+v, want only the source that still exists (%v)", got, kept)
	}
}

// A source that stops opening changes the proposal's hash, so the next reading
// supersedes a card that still cites it instead of joining it.
func TestASourceThatStopsOpeningChangesTheProposal(t *testing.T) {
	source := ids.NewV7()
	hashOf := func(missing bool) string {
		facts := deals.StageProgressionFacts{Criteria: []deals.ProgressionCriterion{{
			Label:   "Buyer confirmed the problem",
			Sources: []deals.EvidenceSource{{SourceType: "activity", SourceID: source, Snippet: "Yes.", Missing: missing}},
		}}}
		facts.Criteria[0].Met = true
		raw, err := json.Marshal(progressionChange(facts, textlang.English))
		if err != nil {
			t.Fatalf("rendering the change: %v", err)
		}
		_, hash, err := diffhash.Canonical(raw)
		if err != nil {
			t.Fatalf("hashing the change: %v", err)
		}
		return hash
	}
	if hashOf(false) == hashOf(true) {
		t.Error("a proposal whose only source stopped opening hashes the same, so it joins the stale card")
	}
}
