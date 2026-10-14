// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The deal object's own write path.
//
// A deal row brings its history: it may already be won or lost, closed on a day
// in the past. The deals store owns how such a deal is born and moved. This file
// decides what a row asks of it, and answers each refusal as a skipped row with
// its reason, never as a failed run.

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// relationshipKindStakeholder is the edge a deal's contact_ref means.
const relationshipKindStakeholder = "deal_stakeholder"

// createDeal lands one row as a new deal. An open deal lands on the stage the
// file names. A won or lost one is born open and closed in the same
// transaction. A contact_ref lands as a stakeholder in that transaction.
func (w *csvWriters) createDeal(ctx context.Context, row migration.Row, typed map[string]any) (migration.EnsureResult, error) {
	made := dealCreateFrom(textFields(row.Fields), typed, w.sourceSystem, w.provenanceOf(row.ExternalID))
	if made.Deal.StageID.IsZero() {
		return skippedFor(migration.Why(migration.ReasonStageUnnamed)), nil
	}
	terminal := w.terminalOf(textFields(row.Fields), made)
	err := w.land(ctx, row, func(tx pgx.Tx) (ids.UUID, error) {
		deal, err := w.deals.ImportDealTx(ctx, tx, made.Deal, terminal, w.cols.deals)
		if err != nil {
			return ids.UUID{}, fmt.Errorf("import: creating deal %s: %w", row.ExternalID, err)
		}
		id := ids.UUID(deal.Id)
		if err := w.seatStakeholder(ctx, tx, id, typed, w.provenanceOf(row.ExternalID)); err != nil {
			return ids.UUID{}, err
		}
		return id, nil
	})
	if why, refused := dealRefusal(err, textFields(row.Fields)); refused {
		return skippedFor(why), nil
	}
	if err != nil {
		return migration.EnsureResult{}, err
	}
	return migration.EnsureResult{Created: true}, nil
}

// terminalOf is the closed stage a new deal ends in, nil for an open one. The
// stage's semantic is not in typed, so it is read from the same pipelines the
// cell check read.
func (w *csvWriters) terminalOf(fields map[string]string, made dealCreate) *deals.ImportTerminal {
	choice, problem := w.dealStages.resolve(fields[dealTargetPipeline], fields[dealTargetStage])
	if problem != nil || choice.semantic == crmcontracts.StageSemanticOpen {
		return nil
	}
	return &deals.ImportTerminal{StageID: choice.stage, ClosedAt: made.Deal.ExpectedClose, LostReason: made.LostReason}
}

// seatStakeholder writes the stakeholder edge a row's contact_ref names, in the
// caller's transaction, through the same writer a lead's promotion uses. A row
// with no resolved contact writes nothing.
func (w *csvWriters) seatStakeholder(ctx context.Context, tx pgx.Tx, dealID ids.UUID, typed map[string]any, source string) error {
	contact, ok := linkedRecord(typed, dealTargetContactRef)
	if !ok {
		return nil
	}
	seated := ids.From[ids.ContactKind](contact)
	deal := ids.From[ids.DealKind](dealID)
	if _, err := w.contacts.CreateRelationshipTx(ctx, tx, contacts.CreateRelationshipInput{
		Kind: relationshipKindStakeholder, ContactID: &seated, DealID: &deal, Source: source,
	}); err != nil {
		return fmt.Errorf("import: seating the contact on the deal: %w", err)
	}
	return nil
}

// dealRefusal answers the store's refusals of one deal row as the reason the
// report shows. Anything it does not know is not a bad row and stops the run.
func dealRefusal(err error, fields map[string]string) (migration.Reason, bool) {
	if err == nil {
		return migration.Reason{}, false
	}
	if _, ok := errors.AsType[*deals.PastCloseDateError](err); ok {
		return migration.Why(migration.ReasonCloseDatePast, "value", strings.TrimSpace(fields[dealTargetCloseDate])), true
	}
	if _, ok := errors.AsType[*deals.StagePipelineMismatchError](err); ok {
		return migration.Why(migration.ReasonDealPipelineFixed), true
	}
	if _, ok := errors.AsType[*deals.LostReasonRequiredError](err); ok {
		return migration.Why(migration.ReasonLostReasonMissing), true
	}
	if missing, ok := errors.AsType[*deals.MissingFxRateError](err); ok {
		return migration.Why(migration.ReasonFxRateMissing, "currency", missing.From), true
	}
	if _, ok := errors.AsType[*deals.LegalReviewRequiredError](err); ok {
		return migration.Why(migration.ReasonDealLegalGate), true
	}
	if _, ok := errors.AsType[*deals.LegalGateOnCreateError](err); ok {
		return migration.Why(migration.ReasonDealLegalGate), true
	}
	if _, ok := errors.AsType[*deals.PipelineHasNoOpenStageError](err); ok {
		return migration.Why(migration.ReasonPipelineNoOpenStage), true
	}
	return migration.Reason{}, false
}
