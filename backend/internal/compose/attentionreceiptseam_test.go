// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/attention"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
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
		if receipt.Source != attention.ReceiptFromCorrection {
			t.Errorf("a %s receipt is not marked as a correction, so the sign-in page offers it as an approval", receipt.Kind)
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

func TestEveryCorrectionReceiptNamesItsAuditEntryEvenWithoutAnUndo(t *testing.T) {
	t.Parallel()
	for _, correction := range []string{
		deals.CloseLostCorrection, deals.VerdictStageMoveCorrection, deals.DealOpenCorrection,
		deals.DealReviveCorrection, deals.VerdictAmountCorrection, deals.VerdictProbabilityCorrection,
		string(deals.CloseDateOverdue),
	} {
		audit := ids.NewV7()
		receipt := correctionReceipt(deals.CorrectionReceipt{Correction: correction, AuditLogID: audit, Fields: []string{"expected_close_date"}})
		receipt.Undo = nil
		wire := attention.WireReceipt(receipt)
		if wire.Source == nil || *wire.Source != crmcontracts.ReceiptSourceCorrection {
			t.Errorf("a %s receipt names no correction source: %v", correction, wire.Source)
		}
		if wire.AuditLogId == nil || ids.UUID(*wire.AuditLogId) != audit {
			t.Errorf("a %s receipt without an Undo lost its audit entry: %v", correction, wire.AuditLogId)
		}
	}
}

func TestAnApprovalReceiptCarriesNoAuditEntry(t *testing.T) {
	t.Parallel()
	wire := attention.WireReceipt(attention.Receipt{Source: attention.ReceiptFromApproval, ID: ids.NewV7(), Kind: "send_email"})
	if wire.Source == nil || *wire.Source != crmcontracts.ReceiptSourceApproval || wire.AuditLogId != nil {
		t.Fatalf("an approval receipt must say approval and name no audit entry: %v %v", wire.Source, wire.AuditLogId)
	}
}

func TestADecidedCorrectionOffersTheUndoItsReviewAllows(t *testing.T) {
	t.Parallel()
	audit := ids.NewV7()
	correction := func(review crmcontracts.AppliedDealChangeReview, reversed bool) attention.Receipt {
		return attention.Receipt{
			Source: attention.ReceiptFromCorrection, ID: audit, AuditLogID: audit, Review: &review,
			Undo: &attention.ReceiptUndo{AuditLogID: audit, Version: review.Version, Reversed: reversed},
		}
	}
	live := crmcontracts.AppliedDealChangeReview{Version: 7, CanUndo: true, CanAccept: true, Writable: true}
	for name, tc := range map[string]struct {
		receipt attention.Receipt
		reason  Reason
	}{
		"reversed":       {correction(live, true), ReasonAlreadyUndone},
		"not writable":   {correction(crmcontracts.AppliedDealChangeReview{Version: 7, CanUndo: true, CanAccept: true}, false), ReasonNotWritableByCaller},
		"edited since":   {correction(crmcontracts.AppliedDealChangeReview{Version: 7, Writable: true}, false), ReasonSuperseded},
		"without review": {attention.Receipt{Source: attention.ReceiptFromCorrection, Undo: &attention.ReceiptUndo{AuditLogID: audit}}, ReasonNotRestorableByThisPath},
	} {
		undo := decidedUndo(tc.receipt)
		if undo == nil || undo.Undoable || undo.Reason == nil || *undo.Reason != string(tc.reason) {
			t.Errorf("%s: want a refusal %q, got %+v", name, tc.reason, undo)
		}
	}
	offered := decidedUndo(correction(live, false))
	if offered == nil || !offered.Undoable || offered.AuditId == nil || offered.Version == nil || *offered.Version != 7 {
		t.Fatalf("a live correction must offer Undo with its audit entry and version, got %+v", offered)
	}
	if decidedUndo(attention.Receipt{Source: attention.ReceiptFromApproval, ID: audit}) != nil {
		t.Fatal("an approval must carry no Undo on the done lane")
	}
}
