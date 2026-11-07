// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/attention"
	"github.com/margince/margince/backend/internal/modules/deals"
)

func TestAVerdictCloseDateKeepsItsOwnReceiptKind(t *testing.T) {
	t.Parallel()
	for correction, want := range map[string]string{
		deals.VerdictCloseDateCorrection: deals.VerdictCloseDateCorrection,
		string(deals.CloseDateOverdue):   deals.CloseDateCorrectionKind,
	} {
		receipt := correctionReceipt(deals.CorrectionReceipt{Correction: correction, Fields: []string{"expected_close_date"}})
		if receipt.Kind != want || receipt.Undo == nil {
			t.Errorf("receipt of a %s change = kind %q undo %v, want kind %q with an Undo", correction, receipt.Kind, receipt.Undo, want)
		}
		if !deals.IsCorrectionReceiptKind(receipt.Kind) {
			t.Errorf("kind %q is not a correction receipt kind, so the sign-in page offers it as an approval", receipt.Kind)
		}
	}
}

func TestEveryCorrectionReceiptCarriesItsReasonApartFromItsHeadline(t *testing.T) {
	t.Parallel()
	const basis = "the buyer withdrew the budget"
	to := "Closed lost"
	for _, correction := range []string{
		deals.CloseLostCorrection, deals.VerdictStageMoveCorrection, deals.DealOpenCorrection, deals.DealReviveCorrection,
		deals.VerdictAmountCorrection, deals.VerdictProbabilityCorrection, deals.VerdictCloseDateCorrection,
		string(deals.CloseDateOverdue),
	} {
		receipt := correctionReceipt(deals.CorrectionReceipt{
			Correction: correction, DealName: "Acme", Basis: basis, ToStage: &to, Fields: []string{"expected_close_date"},
		})
		if receipt.Reason != basis {
			t.Errorf("%s: reason = %q, want the recorded basis", correction, receipt.Reason)
		}
		if receipt.Headline == "" || strings.Contains(receipt.Headline, basis) {
			t.Errorf("%s: headline = %q, want the change without its reason", correction, receipt.Headline)
		}
		if !strings.Contains(receipt.Summary, basis) {
			t.Errorf("%s: summary = %q, want it to keep the reason for clients that draw only the summary", correction, receipt.Summary)
		}
	}
}

func TestACustomerQuoteTravelsApartFromMargincesReason(t *testing.T) {
	t.Parallel()
	receipt := correctionReceipt(deals.CorrectionReceipt{
		Correction: deals.VerdictAmountCorrection, DealName: "Acme", Basis: "the quote names a total",
		Evidence: []byte(`{"quoted_words":"We can do 45,000 EUR"}`),
	})
	if receipt.CustomerQuote != "We can do 45,000 EUR" || receipt.Reason != "the quote names a total" {
		t.Fatalf("quote %q reason %q, want the customer's words and Margince's reading kept apart", receipt.CustomerQuote, receipt.Reason)
	}
	if strings.Contains(receipt.Headline, "45,000") {
		t.Errorf("headline %q carries the quote, which a client draws on its own line", receipt.Headline)
	}
}

func TestAChangeWithNoRecordedReasonSendsNoHeadline(t *testing.T) {
	t.Parallel()
	to := "Closed lost"
	for _, correction := range []string{
		deals.CloseLostCorrection, deals.VerdictStageMoveCorrection, deals.DealOpenCorrection, deals.DealReviveCorrection,
		deals.VerdictAmountCorrection, deals.VerdictProbabilityCorrection, deals.VerdictCloseDateCorrection,
		string(deals.CloseDateOverdue),
	} {
		wire := attention.WireReceipt(correctionReceipt(deals.CorrectionReceipt{
			Correction: correction, DealName: "Acme", ToStage: &to, Fields: []string{"expected_close_date"},
		}))
		if wire.Headline != nil || wire.Reason != nil || wire.CustomerQuote != nil {
			t.Errorf("%s: headline %v reason %v quote %v, want all absent when nothing was recorded", correction, wire.Headline, wire.Reason, wire.CustomerQuote)
		}
	}
}
