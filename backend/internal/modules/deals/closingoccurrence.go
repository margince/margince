// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// Which closing a closed deal is currently on.
//
// A deal can be closed, reopened and closed again. Each of those closings is a
// different thing to have an opinion about: a loss in March and a win in June
// are two outcomes, and anything written about the first must not silently
// reattach itself to the second.
//
// deal_stage_history already records every move, so the row that put the deal
// into its current terminal stage IS the occurrence. Nothing new identifies it,
// nothing is stored twice, and a reopen makes the old occurrence stop being
// current without deleting or rewriting anything — which is what lets an old
// review stay readable while no longer counting as the review of this close.
//
// It is DERIVED at read rather than stamped on the deal. A column would be a
// second answer to a question history already answers, and the two would
// disagree the first time somebody reverted a stage through a path that forgot
// to update it — which is exactly the class of bug the single-writer rule on
// deal_stage_history exists to prevent.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ClosingOccurrence names one closing: the history row that made it, and the
// outcome that row recorded.
//
// Exported because a review is written by another module, and compose joins the
// two. One type rather than an internal shape and an exported copy: the copy
// would have to be kept in step by hand, and the only thing keeping it honest
// would be that both happen to have the same two fields today.
type ClosingOccurrence struct {
	ID ids.UUID
	// Outcome is the semantic the deal MOVED INTO, read from the stage as it
	// was at the time. A stage can be renamed or reconfigured afterwards; what
	// the deal did cannot.
	Outcome string
}

// currentClosingOccurrence answers the closing a deal is on right now, or
// nothing where the deal is open.
//
// The ordering is changed_at then id, the same pair the trajectory view uses,
// because two moves can share a timestamp — an advance and the reversal that
// undoes it can land in one transaction — and a bare timestamp would pick
// between them arbitrarily. uuidv7 ids sort by creation, so the id breaks the
// tie in the order the rows were actually written.
//
// A deal with no defensible history returns nothing rather than a guess. That
// is a real state: rows imported before this history existed carry no move
// into their terminal stage, and manufacturing an occurrence for them would
// invite a review of a closing nobody can point at.
func currentClosingOccurrence(ctx context.Context, tx pgx.Tx, dealID ids.DealID) (ClosingOccurrence, bool, error) {
	// The deal's row scope, in the reader rather than in its callers.
	//
	// Both callers already hold a deal the caller may read — one is reached
	// from readDealForCaller, the other checks visibility itself — so this is
	// the second check on both paths. That is deliberate: a read bounded only
	// by its callers becomes unbounded the first time a third one appears, and
	// nothing fails at that moment to say so. An occurrence is a fact about
	// WHEN a particular deal closed, which is a disclosure about the deal.
	if err := auth.EnsureVisible(ctx, tx, dealTable, dealID.UUID); err != nil {
		return ClosingOccurrence{}, false, err
	}

	var out ClosingOccurrence
	// Read off the FROZEN semantic, never off the live stage. A stage's
	// semantic is configuration an administrator may edit, so joining it here
	// would report an old closing as whatever the stage means today — or lose
	// it, once the deal's status and the stage's current meaning disagree.
	//
	// A row with no frozen semantic is one written before the column existed.
	// It answers nothing rather than being guessed at, which reads as "this
	// deal is on no closing anybody can point at" — honest, and exactly the
	// state the review path already refuses.
	err := tx.QueryRow(ctx,
		`SELECT h.id, h.semantic_at_change
		   FROM deal_stage_history h
		   JOIN deal d ON d.id = h.deal_id
		  WHERE h.deal_id = $1
		    AND d.status <> 'open'
		    AND h.semantic_at_change = d.status
		  ORDER BY h.changed_at DESC, h.id DESC
		  LIMIT 1`, dealID).Scan(&out.ID, &out.Outcome)
	if errors.Is(err, pgx.ErrNoRows) {
		return ClosingOccurrence{}, false, nil
	}
	if err != nil {
		return ClosingOccurrence{}, false, fmt.Errorf("read the deal's current closing: %w", err)
	}
	return out, true, nil
}

// attachClosingOccurrence puts the deal's current closing on the wire shape,
// leaving it absent where the deal is open or its history cannot say.
func attachClosingOccurrence(ctx context.Context, tx pgx.Tx, id ids.DealID, d *crmcontracts.Deal) error {
	occurrence, found, err := currentClosingOccurrence(ctx, tx, id)
	if err != nil {
		return err
	}
	if found {
		occurrenceID := openapi_types.UUID(occurrence.ID)
		d.ClosingOccurrenceId = &occurrenceID
	}
	return nil
}

// HoldClosingOccurrenceTx answers the closing a deal is on, under the caller's
// transaction, having first checked the caller may read the deal at all.
//
// Exported because a review is written by another module and compose joins the
// two. It carries the authority check rather than leaving it to the caller: the
// occurrence identifies a closing of a particular deal, so answering it to
// somebody who cannot see that deal would confirm the deal exists and say when
// it closed.
//
// It refuses an OPEN deal rather than answering nothing, because a caller
// asking for the occurrence is about to write something about an outcome, and
// "there is no outcome yet" is a different answer from "you may not see it".
func (s *Store) HoldClosingOccurrenceTx(ctx context.Context, tx pgx.Tx, id ids.DealID) (ClosingOccurrence, error) {
	// The RBAC OBJECT, spelled out. dealTable happens to read the same and
	// means something else — one is a relation name, the other a permission's
	// vocabulary — and passing the table here would tie a permission to a
	// rename that has nothing to do with it.
	// The RBAC OBJECT, spelled out. dealTable happens to read the same and
	// means something else — one is a relation name, the other a permission's
	// vocabulary — and passing the table here would tie a permission to a
	// rename that has nothing to do with it.
	//
	// The row scope is the reader's own, below.
	if err := auth.Require(ctx, "deal", principal.ActionRead); err != nil {
		return ClosingOccurrence{}, err
	}
	// HOLD, which is what the name promises and what the caller needs.
	//
	// The occurrence this answers is about to be written into a review, and an
	// unlocked read would let the deal be reopened and re-closed between the
	// answer and the write. The composite foreign key would still accept the
	// old closing — it is a real closing of this deal — so the review would
	// commit against an outcome the deal had already left, with nothing
	// failing to say so. The lock holds the deal still until the caller's
	// transaction ends.
	if _, err := storekit.LockRow(ctx, tx, dealTable, id.UUID, storekit.LiveOnly); err != nil {
		return ClosingOccurrence{}, err
	}
	occurrence, found, err := currentClosingOccurrence(ctx, tx, id)
	if err != nil {
		return ClosingOccurrence{}, err
	}
	if !found {
		return ClosingOccurrence{}, &DealNotClosedError{}
	}
	return occurrence, nil
}

// DealNotClosedError refuses to answer for a closing that has not happened.
//
// It covers two states on purpose, because they are one answer to the caller:
// a deal still open, and a closed deal whose history does not record the move
// that closed it. In both, there is no closing anybody can point at, and a
// review filed against one would name nothing.
type DealNotClosedError struct{}

func (e *DealNotClosedError) Error() string {
	return "this deal is not on a closing anybody can point at, so there is no outcome to review"
}

// FieldFault classifies the refusal as a 422 naming the occurrence.
func (e *DealNotClosedError) FieldFault() (field, code, message string) {
	return "closing_occurrence_id", "deal_not_closed", e.Error()
}

// StaleClosingError refuses a review filed against a closing the deal has since
// left. The deal was reopened and re-closed between the form opening and the
// submission, so the review would attach to an outcome that is no longer the
// one this deal is on.
type StaleClosingError struct{ Current ids.UUID }

func (e *StaleClosingError) Error() string {
	return "this deal has closed again since the review was started, so the review would be about the wrong outcome"
}

// Unwrap makes this a 409 through the sentinel registry rather than through a
// spelling of its own in a transport. Nothing about the request is malformed
// and no rewording would fix it: the deal moved underneath the caller, and the
// answer is to reload and look at the outcome it is on now.
func (e *StaleClosingError) Unwrap() error { return apperrors.ErrConflict }
