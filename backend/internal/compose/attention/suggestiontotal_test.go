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

// A suggestion lane cut at the read depth states the store's open count as
// the decisions total, in each scope that reads suggestions.
func TestTheDecisionsTotalIsTheExactSuggestionCountPastTheReadDepth(t *testing.T) {
	for _, scope := range []string{scopeAll, scopeUnassigned} {
		t.Run(scope, func(t *testing.T) {
			day := crmcontracts.Attention{
				AsOf: rankInstant,
				NeedsYou: append(suggestionItems(batchScanDepth/2, sourceDealSuggestion),
					suggestionItems(batchScanDepth/2, sourceTagSuggestion)...),
			}
			day.Counts.ApprovalsOpen = new(0)
			day.Counts.DealSuggestionsOpen = new(batchScanDepth/2 + 30)
			day.Counts.TagSuggestionsOpen = new(batchScanDepth/2 + 7)

			got := (&Service{}).worklistFrom(t.Context(), day, scope, "", 25, waitingRead{}, worklistCursor{}, nil)

			count := countFor(t, got, "decisions")
			if want := batchScanDepth + 37; count.Total == nil || *count.Total != want {
				t.Fatalf("decisions total = %v, want both stores' exact %d", count.Total, want)
			}
			if !count.MoreAvailable {
				t.Error("the page read part of each suggestion source and claimed it considered them all")
			}
		})
	}
}
