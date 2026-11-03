// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"slices"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

func TestOnlyCurrentCommercialContextOffersADecision(t *testing.T) {
	for _, state := range []string{"ready", "stale", "incomplete", "unavailable"} {
		item := approvalItem(crmcontracts.Approval{Kind: "advance_deal", Preview: &crmcontracts.ApprovalPreview{Version: 1, State: state}}, nil)
		if got := slices.Contains(item.Actions, "decide"); got != (state == "ready") {
			t.Errorf("%s offers a decision: %v", state, got)
		}
	}
}

// The row and the receipt carry the summary taken apart. The client then shows
// the proposed values as a list, not as the joined key=value line.
func TestTheApprovalRowAndReceiptCarryTheSummaryParts(t *testing.T) {
	summary := "Update record deal: name=Acme, Inc."
	parts := &crmcontracts.ApprovalSummaryParts{
		Sentence: "Update record deal", Fields: []crmcontracts.ApprovalSummaryField{{Field: "name", Value: "Acme, Inc."}},
	}
	row := approvalItem(crmcontracts.Approval{Kind: "update_record", Summary: &summary, SummaryParts: parts}, nil)
	if row.SummaryParts != parts || row.Title == nil || *row.Title != summary {
		t.Errorf("approval row carries title %v and parts %+v, want the summary and its parts", row.Title, row.SummaryParts)
	}
	if worklist := base(row, 0, "", ""); worklist.SummaryParts != parts {
		t.Errorf("worklist row carries parts %+v, want %+v", worklist.SummaryParts, parts)
	}
	receipt := Receipt{Kind: "update_record", Summary: summary, SummaryParts: parts}
	if item := receiptItem(receipt); item.SummaryParts != parts {
		t.Errorf("receipt row carries parts %+v, want %+v", item.SummaryParts, parts)
	}
	if handled := handledReceipt(receipt); handled.SummaryParts != parts {
		t.Errorf("handled receipt carries parts %+v, want %+v", handled.SummaryParts, parts)
	}
}
