// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"encoding/json"
	"fmt"

	"github.com/margince/margince/backend/internal/compose/attention"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// verdictReceipt renders an amount or probability Margince set from the
// deal's evidence, with the value before and after and the reason. An amount
// also names the quote and the conversion.
func verdictReceipt(c deals.CorrectionReceipt) attention.Receipt {
	var summary string
	if c.Correction == deals.VerdictAmountCorrection {
		summary = fmt.Sprintf("Changed the amount on %q: %s → %s", c.DealName,
			receiptAmount(c.BeforeAmount, c.Currency), receiptAmount(c.AfterAmount, c.Currency))
	} else {
		summary = fmt.Sprintf("Changed the win probability on %q: %s → %s", c.DealName,
			receiptProbability(c.BeforeProbability), receiptProbability(c.AfterProbability))
	}
	if c.Basis != "" {
		summary += " — " + c.Basis
	}
	summary += amountSource(c.Evidence)
	return attention.Receipt{
		ID:         c.AuditLogID,
		Kind:       c.Correction,
		Summary:    summary,
		OccurredAt: c.AppliedAt,
		TargetType: approvalTargetDeal,
		TargetID:   c.DealID.UUID,
		Actor:      attention.ReceiptActor{Type: c.ActorType, ID: c.ActorID, OnBehalfOf: c.OnBehalfOf},
		Undo:       &attention.ReceiptUndo{AuditLogID: c.AuditLogID, Version: c.Version, Reversed: c.Reversed},
	}
}

// stageMoveReceipt renders a deal Margince moved to a later stage, with the
// reason and the probability the deal took with the stage.
func stageMoveReceipt(c deals.CorrectionReceipt) attention.Receipt {
	summary := fmt.Sprintf("Moved %q from %s to %s", c.DealName, receiptStage(c.FromStage), receiptStage(c.ToStage))
	if c.Basis != "" {
		summary += ": " + c.Basis
	}
	if c.StageProbability != nil {
		summary += fmt.Sprintf(". Probability now %d%%", *c.StageProbability)
	}
	return attention.Receipt{
		ID:         c.AuditLogID,
		Kind:       deals.DealStageMoveKind,
		Summary:    summary,
		OccurredAt: c.AppliedAt,
		TargetType: approvalTargetDeal,
		TargetID:   c.DealID.UUID,
		Actor:      attention.ReceiptActor{Type: c.ActorType, ID: c.ActorID, OnBehalfOf: c.OnBehalfOf},
		Undo:       &attention.ReceiptUndo{AuditLogID: c.AuditLogID, Version: c.Version, Reversed: c.Reversed},
	}
}

// dealReviveReceipt renders a lost deal Margince reopened for its owner, with
// the stage it reopened in and the new buying evidence.
func dealReviveReceipt(c deals.CorrectionReceipt) attention.Receipt {
	summary := fmt.Sprintf("Reopened %q", c.DealName)
	if c.ToStage != nil {
		summary += " in " + *c.ToStage
	}
	if c.Basis != "" {
		summary += ": " + c.Basis
	}
	return attention.Receipt{
		ID:         c.AuditLogID,
		Kind:       deals.DealReviveKind,
		Summary:    summary,
		OccurredAt: c.AppliedAt,
		TargetType: approvalTargetDeal,
		TargetID:   c.DealID.UUID,
		Actor:      attention.ReceiptActor{Type: c.ActorType, ID: c.ActorID, OnBehalfOf: c.OnBehalfOf},
		Undo:       &attention.ReceiptUndo{AuditLogID: c.AuditLogID, Version: c.Version, Reversed: c.Reversed},
	}
}

// dealOpenReceipt renders a deal Margince opened for the company's owner, with
// the buying evidence it rests on.
func dealOpenReceipt(c deals.CorrectionReceipt) attention.Receipt {
	summary := fmt.Sprintf("Opened the deal %q", c.DealName)
	if c.Basis != "" {
		summary += ": " + c.Basis
	}
	return attention.Receipt{
		ID:         c.AuditLogID,
		Kind:       deals.DealOpenKind,
		Summary:    summary,
		OccurredAt: c.AppliedAt,
		TargetType: approvalTargetDeal,
		TargetID:   c.DealID.UUID,
		Actor:      attention.ReceiptActor{Type: c.ActorType, ID: c.ActorID, OnBehalfOf: c.OnBehalfOf},
		Undo:       &attention.ReceiptUndo{AuditLogID: c.AuditLogID, Version: c.Version, Reversed: c.Reversed},
	}
}

func receiptStage(name *string) string {
	if name == nil {
		return "a stage since removed"
	}
	return *name
}

func receiptAmount(amount *int64, currency *string) string {
	if amount == nil || currency == nil {
		return "none"
	}
	return values.MajorUnits(*amount, *currency) + " " + *currency
}

func receiptProbability(pct *int) string {
	if pct == nil {
		return "the stage's"
	}
	return fmt.Sprintf("%d%%", *pct)
}

// amountSource names the customer's words and, for another currency, the
// amount they named and the rate's date.
func amountSource(evidence []byte) string {
	var facts struct {
		Quote    string `json:"quoted_words"`
		Amount   string `json:"quoted_amount"`
		Currency string `json:"quoted_currency"`
		RateDate string `json:"rate_date"`
	}
	if len(evidence) == 0 || json.Unmarshal(evidence, &facts) != nil || facts.Quote == "" {
		return ""
	}
	out := fmt.Sprintf(". The customer wrote %q", facts.Quote)
	if facts.RateDate != "" {
		out += fmt.Sprintf("; %s %s converted at the rate of %s", facts.Amount, facts.Currency, facts.RateDate)
	}
	return out
}
