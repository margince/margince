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

	"github.com/margince/margince/backend/internal/platform/settings"
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
	for _, r := range req.Recipients {
		if err := r.Validate(); err != nil {
			return commsauthz.DecisionSet{}, fmt.Errorf(
				"consent: this recipient cannot be put to the engine: %w", err)
		}
	}
	if len(req.Recipients) == 0 {
		return commsauthz.DecisionSet{}, fmt.Errorf(
			"consent: a preview needs at least one recipient: %w", apperrors.ErrInvalidArgument)
	}

	var set commsauthz.DecisionSet
	err := g.store.db.Tx(ctx, func(tx pgx.Tx) error {
		modes, err := settings.ApplyTx(ctx, tx, AuthorizationModes)
		if err != nil {
			return err
		}
		for _, r := range req.Recipients {
			d, err := g.decideOne(ctx, tx, r, req, commsauthz.PhaseStaging)
			if err != nil {
				return err
			}
			// PhaseStaging is passed to decideOne and then cleared, and the two
			// are not in tension. The phase steers what the DECISION does —
			// staging is the phase that records a basis — so asking as staging
			// is what makes the preview answer the same question the staging
			// decision will. Leaving it on the returned decision would be the
			// lie: nothing here is a staging decision, because nothing here is
			// recorded.
			d.Phase = ""
			d.Mode = ModeFor(modes, d.Resolved)
			d.Requested = req.Context
			set.Decisions = append(set.Decisions, d)
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

// errPreviewComplete unwinds Preview's transaction once it has its answer.
//
// Sentinel rather than a bare errors.New at the call site so nothing can
// mistake it for a fault: it never leaves this file's function, and a reader
// who greps it finds exactly one producer and one consumer.
var errPreviewComplete = errors.New("consent: preview complete")
