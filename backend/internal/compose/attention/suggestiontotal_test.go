// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"fmt"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

func suggestionItems(count int, source crmcontracts.AttentionItemSource) []crmcontracts.AttentionItem {
	rows := make([]crmcontracts.AttentionItem, 0, count)
	for i := range count {
		rows = append(rows, item(fmt.Sprintf("%s-%d", source, i), source, decidable()))
	}
	return rows
}

// Scrolling ends at the ranked rows for suggestions, so a cut suggestion lane
// leaves the decisions category a floor, though the store counted every one.
func TestACutSuggestionLaneLeavesTheDecisionsAFloor(t *testing.T) {
	for _, source := range []crmcontracts.AttentionItemSource{sourceDealSuggestion, sourceTagSuggestion} {
		t.Run(string(source), func(t *testing.T) {
			day := crmcontracts.Attention{AsOf: rankInstant, NeedsYou: suggestionItems(batchScanDepth, source)}
			day.Counts.ApprovalsOpen = new(0)
			open := batchScanDepth + 30
			if source == sourceDealSuggestion {
				day.Counts.DealSuggestionsOpen = &open
			} else {
				day.Counts.TagSuggestionsOpen = &open
			}

			got := (&Service{}).worklistFrom(t.Context(), day, scopeAll, "", 25, waitingRead{}, worklistCursor{}, nil)

			count := countFor(t, got, "decisions")
			if count.Total != nil {
				t.Errorf("decisions total = %d, but the walk reaches only %d suggestions", *count.Total, batchScanDepth)
			}
			if !count.MoreAvailable {
				t.Error("the page read part of the suggestions and claimed it considered them all")
			}
		})
	}
}
