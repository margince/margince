// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Margince's latest nightly reading of a deal that changed nothing, for the
// deal's "Changes by Margince" card.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/dealverdict"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// checkKinds names each verdict action kind on the wire.
var checkKinds = map[string]crmcontracts.DealMarginceCheckOutcomeKind{
	dealverdict.KindSetCloseDate:   crmcontracts.DealMarginceCheckOutcomeKindCloseDate,
	dealverdict.KindSetAmount:      crmcontracts.DealMarginceCheckOutcomeKindAmount,
	dealverdict.KindSetProbability: crmcontracts.DealMarginceCheckOutcomeKindProbability,
	dealverdict.KindMoveStage:      crmcontracts.DealMarginceCheckOutcomeKindStage,
}

// checkWhys names each stored outcome a reader may be told. An outcome not
// listed reads as other.
var checkWhys = map[string]crmcontracts.DealMarginceCheckOutcomeWhy{
	dealverdict.DroppedSwitchOff:        crmcontracts.DealMarginceCheckOutcomeWhySwitchedOff,
	dealverdict.DroppedHumanEdit:        crmcontracts.DealMarginceCheckOutcomeWhyKeptYourEdit,
	dealverdict.DroppedHumanSet:         crmcontracts.DealMarginceCheckOutcomeWhyKeptYourEdit,
	dealverdict.DroppedLowConfidence:    crmcontracts.DealMarginceCheckOutcomeWhyNotSure,
	dealverdict.DroppedQuoteNotFound:    crmcontracts.DealMarginceCheckOutcomeWhyNotSure,
	dealverdict.DroppedAmountMismatch:   crmcontracts.DealMarginceCheckOutcomeWhyNotSure,
	dealverdict.DroppedDateNotStated:    crmcontracts.DealMarginceCheckOutcomeWhyNotSure,
	dealverdict.DroppedFutureCitation:   crmcontracts.DealMarginceCheckOutcomeWhyNotSure,
	dealverdict.DroppedDateOutOfRange:   crmcontracts.DealMarginceCheckOutcomeWhyOutsideLimits,
	dealverdict.DroppedAboveStage:       crmcontracts.DealMarginceCheckOutcomeWhyOutsideLimits,
	deals.VerdictDroppedStale:           crmcontracts.DealMarginceCheckOutcomeWhyChangedWhileChecking,
	deals.VerdictDroppedMoved:           crmcontracts.DealMarginceCheckOutcomeWhyChangedWhileChecking,
	deals.VerdictDroppedOwnerCannotEdit: crmcontracts.DealMarginceCheckOutcomeWhyOwnerCannotEdit,
	dealverdict.DroppedUndoHold:         crmcontracts.DealMarginceCheckOutcomeWhyUndoHold,
	dealverdict.DroppedUnchanged:        crmcontracts.DealMarginceCheckOutcomeWhyAlreadyRight,
	dealverdict.DroppedOfferFixes:       crmcontracts.DealMarginceCheckOutcomeWhyAcceptedOffer,
	dealverdict.DroppedNoRate:           crmcontracts.DealMarginceCheckOutcomeWhyNoRate,
	droppedNoOwner:                      crmcontracts.DealMarginceCheckOutcomeWhyNoOwner,
	dealverdict.DroppedNotForward:       crmcontracts.DealMarginceCheckOutcomeWhyStageRules,
	dealverdict.DroppedCriterionUnmet:   crmcontracts.DealMarginceCheckOutcomeWhyStageRules,
	dealverdict.DroppedNoExitCriteria:   crmcontracts.DealMarginceCheckOutcomeWhyStageRules,
	deals.VerdictDroppedStageRefused:    crmcontracts.DealMarginceCheckOutcomeWhyStageRules,
}

// latestCheck reads the deal's latest verdict when it applied nothing. It is
// nil when there is none or it changed something. Why, and which sources were
// read, leave the database only for the owner the verdict was made for.
func latestCheck(ctx context.Context, db *database.DB, dealID ids.UUID) (*crmcontracts.DealMarginceCheck, error) {
	if err := auth.Require(ctx, "deal", principal.ActionRead); err != nil {
		return nil, err
	}
	var out *crmcontracts.DealMarginceCheck
	err := db.Tx(ctx, func(tx pgx.Tx) error {
		if err := auth.EnsureVisible(ctx, tx, entityTypeDeal, dealID); err != nil {
			return err
		}
		var check crmcontracts.DealMarginceCheck
		var explained json.RawMessage
		err := tx.QueryRow(ctx, storekit.SQLf(`
			SELECT r.verdict_assessed_at, r.verdict_next_due_at,
			       CASE WHEN %s THEN r.verdict_actions END
			  FROM assurance_deal_receipt r JOIN deal d ON d.id = r.deal_id
			 WHERE r.deal_id = $1 AND r.verdict_assessed_at IS NOT NULL
			   AND NOT %s @> $3::jsonb`,
			deals.BasisReadableSQL("r.verdict_actions->>'for'", "d", 2), verdictActionsSQL("r.verdict_actions")),
			dealID, deals.ReaderOf(ctx), `[{"outcome":"`+deals.VerdictApplied+`"}]`).
			Scan(&check.CheckedAt, &check.NextCheckAt, &explained)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("compose: reading Margince's latest check of the deal: %w", err)
		}
		if err := explainCheck(&check, explained); err != nil {
			return err
		}
		out = &check
		return nil
	})
	return out, err
}

// explainCheck fills why from the stored verdict, or leaves the check
// unexplained when the reader may not read it.
func explainCheck(check *crmcontracts.DealMarginceCheck, raw json.RawMessage) error {
	check.Outcomes, check.Sources = []crmcontracts.DealMarginceCheckOutcome{}, []crmcontracts.DealMarginceCheckSource{}
	if len(raw) == 0 {
		return nil
	}
	stored, err := decodeVerdictRecord(raw)
	if err != nil {
		return fmt.Errorf("compose: decoding Margince's latest check: %w", err)
	}
	check.Explained = true
	if stored.Skipped != "" {
		skipped := crmcontracts.DealMarginceCheckSkipped(stored.Skipped)
		check.Skipped = &skipped
	}
	for _, action := range stored.Actions {
		kind, ok := checkKinds[action.Kind]
		if !ok {
			continue
		}
		check.Outcomes = append(check.Outcomes, crmcontracts.DealMarginceCheckOutcome{Kind: kind, Why: checkWhy(action.Outcome)})
	}
	for _, source := range stored.Sources {
		check.Sources = append(check.Sources, crmcontracts.DealMarginceCheckSource{
			Source: crmcontracts.DealMarginceCheckSourceSource(source.Source),
			State:  crmcontracts.DealMarginceCheckSourceState(source.State),
		})
	}
	return nil
}

func checkWhy(outcome string) crmcontracts.DealMarginceCheckOutcomeWhy {
	if why, ok := checkWhys[outcome]; ok {
		return why
	}
	return crmcontracts.DealMarginceCheckOutcomeWhyOther
}
