// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"testing"

	"github.com/margince/margince/backend/internal/compose/worklistsnap"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A brief row standing alone above the tail already reports its deal, so the
// tail leaves that deal's at-risk row out. A task about a deal does not, and a
// row the ranked top does not hold reports nothing.
func TestTheTailLeavesOutADealARankedRowReports(t *testing.T) {
	briefed, waited, unheld, tasked := ids.NewV7(), ids.NewV7(), ids.NewV7(), ids.NewV7()
	row := func(source crmcontracts.WorklistItemSource, id string, deal ids.UUID) ranked {
		subject := subjectOf(subjectDeal, deal)
		return ranked{item: crmcontracts.WorklistItem{Source: source, Id: id, Subject: subject}}
	}
	ranking := []ranked{
		row(crmcontracts.WorklistItemSourceBriefItem, ids.NewV7().String(), briefed),
		row(crmcontracts.WorklistItemSourceCustomerWaiting, ids.NewV7().String(), waited),
		row(crmcontracts.WorklistItemSourceDealAtRisk, unheld.String(), unheld),
		row(crmcontracts.WorklistItemSourceTask, ids.NewV7().String(), tasked),
	}
	above := map[worklistsnap.Row]bool{
		rowIdentity(ranking[0]): true, rowIdentity(ranking[1]): true, rowIdentity(ranking[3]): true,
	}
	reported := dealsReported(ranking, above)
	if !reported[briefed] || !reported[waited] || reported[unheld] || len(reported) != 2 {
		t.Fatalf("deals reported above: %v, want the briefed and the waited-on deal only", reported)
	}
}
