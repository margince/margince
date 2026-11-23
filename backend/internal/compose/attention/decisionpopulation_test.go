// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// staleApprovalDay holds a decidable approval, an approval whose preview cannot
// be decided yet, and a duplicate pair the reader may only look at.
func staleApprovalDay(approvalsOpen *int) crmcontracts.Attention {
	day := crmcontracts.Attention{
		AsOf: rankInstant,
		NeedsYou: []crmcontracts.AttentionItem{
			item("fresh", "approval", withKind("send_email"), decidable()),
			item("stale", "approval", withKind("send_email")),
			item("pair", sourceDuplicate, withKind("contact")),
		},
	}
	day.Counts.ApprovalsOpen = approvalsOpen
	day.Counts.DuplicatesOpen = new(1)
	return day
}

// Home reads the decisions total and the Worklist strip reads review. Both
// count a stale approval, which waits on this reader, and leave out a pair
// that waits on someone else.
func TestHomeAndTheWorklistCountOneDecisionsPopulation(t *testing.T) {
	for name, approvalsOpen := range map[string]*int{"counted by the store": new(2), "counted from rows": nil} {
		t.Run(name, func(t *testing.T) {
			got := (&Service{}).worklistFrom(t.Context(), staleApprovalDay(approvalsOpen), scopeAll, "", 25, waitingRead{}, worklistCursor{}, nil)

			total := countFor(t, got, "decisions").Total
			if total == nil || *total != 2 {
				t.Fatalf("decisions total = %v, want both approvals and not the pair", total)
			}
			if got.Readings.Review != *total {
				t.Errorf("the strip reads %d decisions and Home reads %d", got.Readings.Review, *total)
			}
		})
	}
}

// Past the read depth the strip reads the store's exact total, as Home does,
// and not the rows this page read.
func TestTheStripReadsTheExactDecisionsTotalPastTheReadDepth(t *testing.T) {
	day := crmcontracts.Attention{AsOf: rankInstant, NeedsYou: approvalItems(batchScanDepth, "send_email")}
	day.NeedsYou[0].Actions = []crmcontracts.AttentionItemActions{}
	day.Counts.ApprovalsOpen = new(batchScanDepth + 140)

	got := (&Service{}).worklistFrom(t.Context(), day, scopeAll, "", 25, waitingRead{}, worklistCursor{}, nil)

	if got.Readings.Review != batchScanDepth+140 {
		t.Errorf("the strip reads %d decisions, want the exact %d Home reads", got.Readings.Review, batchScanDepth+140)
	}
}
