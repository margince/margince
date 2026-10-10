// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"testing"

	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A card cites only sources a reviewer can open. A claim whose activity was
// deleted still settles its criterion, but its pointer is left off the card.
func TestAProgressionCardDropsACitationWhoseSourceIsGone(t *testing.T) {
	kept, gone := ids.NewV7(), ids.NewV7()
	facts := deals.StageProgressionFacts{Criteria: []deals.ProgressionCriterion{{
		Label: "Buyer confirmed the problem",
		Sources: []deals.EvidenceSource{
			{SourceType: "activity", SourceID: kept, Snippet: "Yes, that is our problem."},
			{SourceType: "activity", SourceID: gone, Snippet: "We agree.", Gone: true},
		},
	}}}
	facts.Criteria[0].Met = true

	got := progressionEvidence(facts)
	if len(got) != 1 || got[0].SourceID != kept {
		t.Fatalf("the card cites %+v, want only the source that still exists (%v)", got, kept)
	}
}
