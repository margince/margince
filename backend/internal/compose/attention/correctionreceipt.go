// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"fmt"
	"slices"

	"github.com/margince/margince/backend/internal/modules/deals"
)

// ReceiptTargetDeal is the target type of a receipt about a deal.
const ReceiptTargetDeal = "deal"

// CorrectionReceipt renders one applied correction as a card.
//
// A changed date names both values; confidence-only changes must not claim
// the date moved. The recorded basis explains why the sweep acted.
func CorrectionReceipt(c deals.CorrectionReceipt) Receipt {
	receipt := correctionReceiptOfKind(c)
	receipt.Source, receipt.AuditLogID = ReceiptFromCorrection, c.AuditLogID
	receipt.Reason, receipt.ReasonKind = c.Basis, c.ReasonKind
	receipt.CitedActivities, receipt.CitedMoments = c.CitedActivities, c.CitedMoments
	receipt.CustomerQuote = quotedFacts(c.Evidence).Quote
	if receipt.Reason == "" && receipt.CustomerQuote == "" {
		receipt.Headline = ""
	}
	return receipt
}

func correctionReceiptOfKind(c deals.CorrectionReceipt) Receipt {
	if c.Correction == deals.CloseLostCorrection {
		return closeLostReceipt(c)
	}
	if c.Correction == deals.VerdictStageMoveCorrection {
		return StageMoveReceipt(c)
	}
	if c.Correction == deals.DealOpenCorrection {
		return dealOpenReceipt(c)
	}
	if c.Correction == deals.DealReviveCorrection {
		return DealReviveReceipt(c)
	}
	if c.Correction == deals.VerdictAmountCorrection || c.Correction == deals.VerdictProbabilityCorrection {
		return verdictReceipt(c)
	}
	summary := fmt.Sprintf("Updated close-date confidence on %q", c.DealName)
	if slices.Contains(c.Fields, "expected_close_date") {
		summary = fmt.Sprintf("Changed the close date on %q: %s → %s", c.DealName, correctionDate(c.BeforeClose), correctionDate(c.AfterClose))
	}
	if correctionMovedForecast(c) {
		summary += ". Win probability also lowered"
	}
	headline := summary
	if c.Basis != "" {
		summary += " — " + c.Basis
	}
	// A date the verdict read from the customer keeps its own kind, so its
	// receipt does not read as the sweep's estimate.
	kind := deals.CloseDateCorrectionKind
	if c.Correction == deals.VerdictCloseDateCorrection {
		kind = c.Correction
	}
	return Receipt{
		ID:              c.AuditLogID,
		CloseDateChange: CorrectionValues(c),
		Kind:            kind,
		Summary:         summary,
		Headline:        headline,
		OccurredAt:      c.AppliedAt,
		TargetType:      ReceiptTargetDeal,
		TargetID:        c.DealID.UUID,
		Actor:           ReceiptActor{Type: c.ActorType, ID: c.ActorID, OnBehalfOf: c.OnBehalfOf},
		Undo: &ReceiptUndo{
			AuditLogID: c.AuditLogID,
			Version:    c.Version,
			Reversed:   c.Reversed,
		},
	}
}

// closeLostReceipt renders a deal Margince closed as lost. Its Undo reopens
// the deal; the receipt has no close-date values to show.
func closeLostReceipt(c deals.CorrectionReceipt) Receipt {
	summary := fmt.Sprintf("Closed %q as lost", c.DealName)
	headline := summary
	if c.Basis != "" {
		summary += " — " + c.Basis
	}
	return Receipt{
		ID:         c.AuditLogID,
		Kind:       deals.DealCloseLostKind,
		Summary:    summary,
		Headline:   headline,
		OccurredAt: c.AppliedAt,
		TargetType: ReceiptTargetDeal,
		TargetID:   c.DealID.UUID,
		Actor:      ReceiptActor{Type: c.ActorType, ID: c.ActorID, OnBehalfOf: c.OnBehalfOf},
		Undo:       &ReceiptUndo{AuditLogID: c.AuditLogID, Version: c.Version, Reversed: c.Reversed},
	}
}

func correctionDate(value *string) string {
	if value == nil {
		return "not set"
	}
	return *value
}
