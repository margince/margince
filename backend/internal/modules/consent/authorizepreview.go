// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// Asking the engine what it would decide, without deciding.
//
// The question exists because a refusal is only useful while somebody can act
// on it. A held draft waits in an approval inbox for days, and the answer that
// matters — is this still sendable — has to reach the approver BEFORE their
// decision commits, not from the release that follows it.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// Preview answers what the engine would decide for this message, and records
// nothing.
//
// It runs the SAME per-recipient decision AuthorizeStagingTx runs, from the
// same decideOne, so a preview and the staging decision that follows cannot
// answer differently about an unchanged record. What it does not do is write:
// no decision rows, and no basis — the basis is the ground a send RELIES on,
// and a message nobody has sent has relied on nothing. That is also why
// commsauthz has no PhasePreview: a preview authorizes nothing, so there is no
// row for a phase to appear on.
//
// It opens its own transaction and rolls it back. Rolling back rather than
// promising read-only is what makes "records nothing" true of code rather than
// of intentions: decideOne reaches stampDerivedBasis on an allow, which writes,
// and a future arm that writes something else would be undone by the same
// rollback rather than discovered by a reader.
func (g *Gate) Preview(ctx context.Context, req commsauthz.Request) (commsauthz.DecisionSet, error) {
	if err := previewable(req); err != nil {
		return commsauthz.DecisionSet{}, err
	}
	var set commsauthz.DecisionSet
	err := g.store.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		if set, err = g.decidePreviewTx(ctx, tx, req); err != nil {
			return err
		}
		// The rollback IS the contract. Returning an error is how pgx.Tx is
		// told to roll back, and errPreviewComplete is not a failure — the
		// caller never sees it.
		return errPreviewComplete
	})
	if err != nil && !errors.Is(err, errPreviewComplete) {
		return commsauthz.DecisionSet{}, err
	}
	return set, nil
}

// PreviewTx is Preview on the caller's transaction, inside a savepoint it
// rolls back, so it sees what the caller's transaction has written.
func (g *Gate) PreviewTx(ctx context.Context, tx pgx.Tx, req commsauthz.Request) (commsauthz.DecisionSet, error) {
	if err := previewable(req); err != nil {
		return commsauthz.DecisionSet{}, err
	}
	savepoint, err := tx.Begin(ctx)
	if err != nil {
		return commsauthz.DecisionSet{}, err
	}
	set, err := g.decidePreviewTx(ctx, savepoint, req)
	if rollback := savepoint.Rollback(ctx); err == nil && rollback != nil {
		return commsauthz.DecisionSet{}, rollback
	}
	return set, err
}

func previewable(req commsauthz.Request) error {
	for _, r := range req.Recipients {
		if err := r.Validate(); err != nil {
			return fmt.Errorf("consent: this recipient cannot be put to the engine: %w", err)
		}
	}
	if len(req.Recipients) == 0 {
		return fmt.Errorf("consent: a preview needs at least one recipient: %w", apperrors.ErrInvalidArgument)
	}
	return nil
}

func (g *Gate) decidePreviewTx(ctx context.Context, tx pgx.Tx, req commsauthz.Request) (commsauthz.DecisionSet, error) {
	var set commsauthz.DecisionSet
	modes, err := authorizationModesTx(ctx, tx)
	if err != nil {
		return set, err
	}
	for _, r := range req.Recipients {
		d, err := g.decideOne(ctx, tx, r, req, commsauthz.PhaseStaging)
		if err != nil {
			return set, err
		}
		// Asked as staging so it answers what staging will; cleared because
		// nothing here is recorded as a staging decision.
		d.Phase = ""
		d.Mode = ModeFor(modes, d.Resolved)
		d.Requested = req.Context
		set.Decisions = append(set.Decisions, d)
	}
	return set, nil
}

// errPreviewComplete unwinds Preview's transaction once it has its answer.
//
// Sentinel rather than a bare errors.New at the call site so nothing can
// mistake it for a fault: it never leaves this file's function, and a reader
// who greps it finds exactly one producer and one consumer.
var errPreviewComplete = errors.New("consent: preview complete")
