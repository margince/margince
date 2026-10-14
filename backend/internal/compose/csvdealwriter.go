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
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// relationshipKindStakeholder is the edge a deal's contact_ref means.
const relationshipKindStakeholder = "deal_stakeholder"

// createDeal lands one row as a new deal. An open deal lands on the stage the
// file names. A won or lost one is born open and closed in the same
// transaction. A contact_ref lands as a stakeholder in that transaction.
func (w *csvWriters) createDeal(ctx context.Context, row migration.Row, typed map[string]any) (migration.EnsureResult, error) {
	made, terminal := w.dealBirthOf(row, typed)
	if made.Deal.StageID.IsZero() {
		return skippedFor(migration.Why(migration.ReasonStageUnnamed)), nil
	}
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
	if why, refused := w.dealRefusal(err, textFields(row.Fields)); refused {
		return skippedFor(why), nil
	}
	if err != nil {
		return migration.EnsureResult{}, err
	}
	return migration.EnsureResult{Created: true}, nil
}

// dealBirthOf is what a row asks the deals store to birth, shared by the commit
// and the dry run so both put the same question to the store.
func (w *csvWriters) dealBirthOf(row migration.Row, typed map[string]any) (dealCreate, *deals.ImportTerminal) {
	made := dealCreateFrom(textFields(row.Fields), typed, w.sourceSystem, w.provenanceOf(row.ExternalID))
	return made, w.terminalOf(textFields(row.Fields), made)
}

// predictDealCreate asks the deals store the refusals a birth meets outside the
// row's cells, without writing: the commit's own function answers, so a closed
// deal in a currency with no rate is skipped in the preview for the reason the
// commit gives.
func (w *csvWriters) predictDealCreate(ctx context.Context, row migration.Row, typed map[string]any) (predictedOutcome, migration.Reason, error) {
	made, terminal := w.dealBirthOf(row, typed)
	if made.Deal.StageID.IsZero() {
		return predictUnwritable, migration.Why(migration.ReasonStageUnnamed), nil
	}
	err := database.WithWorkspaceTx(ctx, w.pool, func(tx pgx.Tx) error {
		_, err := w.deals.RefuseImportBirth(ctx, tx, made.Deal, terminal)
		return err
	})
	if why, refused := w.dealRefusal(err, textFields(row.Fields)); refused {
		return predictUnwritable, why, nil
	}
	return predictCreate, migration.Reason{}, err
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
func (w *csvWriters) dealRefusal(err error, fields map[string]string) (migration.Reason, bool) {
	return dealRefusalOf(err, fields, w.dealStages)
}

// dealRefusalOf is dealRefusal over the stages the run read. A stage archived
// after that read still resolves from it and is refused by the store; the row
// is answered as the stage it no longer is.
func dealRefusalOf(err error, fields map[string]string, stages dealStages) (migration.Reason, bool) {
	if err == nil {
		return migration.Reason{}, false
	}
	if _, ok := errors.AsType[*deals.StageGoneError](err); ok {
		return stages.goneStage(fields[dealTargetPipeline], fields[dealTargetStage]), true
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
