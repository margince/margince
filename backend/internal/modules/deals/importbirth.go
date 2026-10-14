// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// The birth of a deal that already has a history. An import brings a deal in as
// the source left it, won or lost, closed on a day in the past. The live doors
// refuse that, so it has a door of its own, and the door is the importer's.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ImportTerminal is the closed stage an imported deal ends in. ClosedAt is the
// day the source closed it; nil closes it now. LostReason is required on a
// lost stage, as it is for a live advance.
type ImportTerminal struct {
	StageID    ids.StageID
	ClosedAt   *time.Time
	LostReason *string
}

// importedWinReason says why an imported win has no agreement behind it: the
// agreement lives in the system the file came from.
var importedWinReason = string(crmcontracts.DealWonWithoutContractReasonImported)

// ImportDealTx births a deal for an import inside the caller's transaction.
//
// An open deal is created like any other, on the stage the file names. A closed
// one is born on the pipeline's first open stage, then advanced through
// advanceOnTx, where a live close writes its fields. The close date may lie in
// the past, and the close lands on the day the source recorded. The live doors
// still refuse a past close date on an open deal.
func (s *Store) ImportDealTx(
	ctx context.Context, tx pgx.Tx, in CreateDealInput, terminal *ImportTerminal, cols CustomColumns,
) (crmcontracts.Deal, error) {
	if !auth.DeclaredImporter(ctx) {
		return crmcontracts.Deal{}, fmt.Errorf("only a declared importer may bring in a closed deal: %w", apperrors.ErrPermissionDenied)
	}
	if err := auth.Require(ctx, "deal", principal.ActionCreate); err != nil {
		return crmcontracts.Deal{}, err
	}
	if len(in.CustomFields) > 0 && len(cols.cols) == 0 {
		return crmcontracts.Deal{}, ErrCustomFieldsNeedTheStoresOwnTransaction
	}
	born, err := s.readyDealCreate(ctx, in)
	if err != nil {
		return crmcontracts.Deal{}, err
	}
	birth := in
	if terminal != nil {
		born.pastCloseAllowed = true
		if birth.StageID, err = firstOpenStage(ctx, tx, in.PipelineID); err != nil {
			return crmcontracts.Deal{}, err
		}
	}
	created, err := s.createDealInTx(ctx, tx, birth, born, cols.cols)
	if err != nil || terminal == nil {
		return created, err
	}
	return s.advanceOnTx(ctx, tx, ids.From[ids.DealKind](ids.UUID(created.Id)), AdvanceDealInput{
		ToStageID:                terminal.StageID,
		LostReason:               terminal.LostReason,
		WonWithoutContractReason: &importedWinReason,
		closedAt:                 terminal.ClosedAt,
	}, cols.cols)
}

// firstOpenStage is the stage a deal that will be closed is born on: the live
// open stage of its pipeline that comes first.
func firstOpenStage(ctx context.Context, tx pgx.Tx, pipelineID ids.PipelineID) (ids.StageID, error) {
	var stage ids.StageID
	err := tx.QueryRow(ctx, `
		SELECT id FROM stage
		 WHERE pipeline_id = $1 AND semantic = 'open' AND archived_at IS NULL
		 ORDER BY position, id LIMIT 1`, pipelineID).Scan(&stage)
	if errors.Is(err, pgx.ErrNoRows) {
		return ids.StageID{}, &PipelineHasNoOpenStageError{}
	}
	if err != nil {
		return ids.StageID{}, fmt.Errorf("read the pipeline's first open stage: %w", err)
	}
	return stage, nil
}

// ImportAdvanceDealTx moves a deal an import already holds to the stage a
// corrected file names, inside the caller's transaction. A move to a closed
// stage lands on the day the file gives, and a win carries the same reason an
// imported birth does. The live advance door has no such parameters.
func (s *Store) ImportAdvanceDealTx(
	ctx context.Context, tx pgx.Tx, id ids.DealID, in AdvanceDealInput, closedAt *time.Time, cols CustomColumns,
) (crmcontracts.Deal, error) {
	if !auth.DeclaredImporter(ctx) {
		return crmcontracts.Deal{}, fmt.Errorf("only a declared importer may close a deal in the past: %w", apperrors.ErrPermissionDenied)
	}
	if err := auth.Require(ctx, "deal", principal.ActionUpdate); err != nil {
		return crmcontracts.Deal{}, err
	}
	in.closedAt = closedAt
	if in.WonWithoutContractReason == nil {
		in.WonWithoutContractReason = &importedWinReason
	}
	return s.advanceOnTx(ctx, tx, id, in, cols.cols)
}
