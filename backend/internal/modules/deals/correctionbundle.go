// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"cmp"
	"context"
	"errors"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

// BundleUndo is one correction of a deal's bundle to take back, and the
// evidence its reversal's audit row carries.
type BundleUndo struct {
	CorrectionID ids.UUID
	Evidence     map[string]any
}

// RevertBundle takes back one deal's corrections in one transaction, in the
// order given, through the reversal the single Undo uses. It answers one
// error per correction: nil where undone, else the refusal that kept it. A
// stale ifVersion or a stage move made since holds the whole deal: it comes
// back as the error, and nothing on the deal changes.
func (s *Store) RevertBundle(
	ctx context.Context, dealID ids.UUID, ifVersion int64, undos []BundleUndo, archiveConfirmedFor string,
) ([]error, error) {
	if err := auth.Require(ctx, "deal", principal.ActionUpdate); err != nil {
		return nil, err
	}
	// Before the transaction, as on the single path: the catalog reads
	// through its own connection.
	active, err := s.activeColumns(ctx)
	if err != nil {
		return nil, err
	}
	kept := make([]error, len(undos))
	err = s.Tx(ctx, func(tx pgx.Tx) error {
		if err := lockBundle(ctx, tx, dealID, undos); err != nil {
			return err
		}
		if err := auth.EnsureWritable(ctx, tx, dealTable, dealID); err != nil {
			return err
		}
		// The deal row is held from here to the commit, so the version is
		// checked once for every reversal after it.
		if err := storekit.EnsureIfMatch(ctx, tx, dealTable, dealID, &ifVersion); err != nil {
			return err
		}
		for i, undo := range undos {
			_, err := storekit.Savepoint(ctx, tx, func(nested pgx.Tx) (bool, error) {
				_, err := s.revertCorrectionTx(ctx, nested, undo.CorrectionID, nil, undo.Evidence, active, archiveConfirmedFor)
				return true, err
			})
			if err != nil && !keepsOneChange(err) {
				return err
			}
			kept[i] = err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return kept, nil
}

// lockBundle takes every named correction of the deal, then the deal.
//
// Correction before deal is the order the single Undo takes. The corrections
// go in id order, so two Undos of one deal wait for each other and never
// deadlock.
func lockBundle(ctx context.Context, tx pgx.Tx, dealID ids.UUID, undos []BundleUndo) error {
	sorted := slices.SortedFunc(slices.Values(undos), func(a, b BundleUndo) int {
		return cmp.Compare(a.CorrectionID.String(), b.CorrectionID.String())
	})
	for _, undo := range sorted {
		correction, err := lockCorrection(ctx, tx, undo.CorrectionID)
		if err != nil {
			return err
		}
		if correction.DealID.UUID != dealID {
			return apperrors.ErrNotFound
		}
	}
	_, err := storekit.LockRow(ctx, tx, dealTable, dealID, storekit.LiveOnly)
	return err
}

// keepsOneChange reports whether a refusal leaves only its own change in
// place, so the rest of the bundle still goes.
func keepsOneChange(err error) bool {
	if _, ok := errors.AsType[*DealOpenUndoNeedsConfirmError](err); ok {
		return true
	}
	conflict, ok := errors.AsType[*CorrectionReversalError](err)
	return ok && !conflict.MovedSince
}

// revertCorrectionTx takes one correction back in the caller's transaction and
// answers the correction it reversed.
func (s *Store) revertCorrectionTx(
	ctx context.Context, tx pgx.Tx, correctionID ids.UUID, ifVersion *int64, evidence map[string]any,
	active []fieldcatalog.Column, archiveConfirmedFor string,
) (DealCorrection, error) {
	// Authority before any conflict. The row is locked first because it names
	// the deal, but nothing about its state reaches a caller before the write
	// check. Otherwise "already taken back" on a deal the caller cannot see
	// would tell them the deal exists, where they are owed a 404.
	correction, err := lockCorrection(ctx, tx, correctionID)
	if err != nil {
		return correction, err
	}
	// The deal row lock comes after the correction lock, the order
	// restoreCorrectedFields already takes, and before the write check.
	if _, err := storekit.LockRow(ctx, tx, dealTable, correction.DealID.UUID, storekit.LiveOnly); err != nil {
		return correction, err
	}
	if err := auth.EnsureWritable(ctx, tx, dealTable, correction.DealID.UUID); err != nil {
		return correction, err
	}
	if correction.Reversed() {
		return correction, &CorrectionReversalError{Reason: alreadyTakenBack}
	}
	// Under the correction lock, so an Accept and an Undo of one change
	// cannot both land, whichever route the Undo came through.
	if correction.AcceptedAt != nil {
		return correction, &CorrectionReversalError{Reason: AlreadyAccepted}
	}
	switch correction.Correction {
	case CloseLostCorrection:
		err = s.revertStageCorrection(ctx, tx, correction, ifVersion, evidence, active, closeMovedSince)
	case VerdictStageMoveCorrection:
		err = s.revertStageCorrection(ctx, tx, correction, ifVersion, evidence, active, stageMoveMovedSince)
	case DealReviveCorrection:
		err = s.revertDealRevive(ctx, tx, correction, ifVersion, evidence, active, archiveConfirmedFor)
	case DealOpenCorrection:
		err = s.revertDealOpen(ctx, tx, correction, ifVersion, archiveConfirmedFor)
	default:
		err = s.restoreCorrectedFields(ctx, tx, correction, ifVersion, evidence)
	}
	return correction, err
}
