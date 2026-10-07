// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const reasonNoNextStep = crmcontracts.WorklistReasonKind("no_next_step")

// A quiet deal with nothing planned says so: the move is to agree the next
// step, not merely to touch the deal. One with a step planned says nothing.
func TestARiskyDealWithNothingPlannedSaysSo(t *testing.T) {
	stepless := oneRiskRow(t, withDealFacts(RiskyDeal{
		DealID: ids.NewV7(), Name: "Fleet retrofit", QuietDays: 19, NoNextStep: true,
	}))
	if !hasReason(stepless, reasonNoNextStep) {
		t.Errorf("the row states %v, want a no_next_step reason", kindsOf(stepless))
	}
	planned := oneRiskRow(t, withDealFacts(RiskyDeal{
		DealID: ids.NewV7(), Name: "Fleet retrofit", QuietDays: 19,
	}))
	if hasReason(planned, reasonNoNextStep) {
		t.Error("a deal with a step planned claims nothing is planned")
	}
}
