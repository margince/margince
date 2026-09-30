// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// The tier that admits the three READINGS ABOUT the queue, as against the queue
// itself. The team board, the hidden-backlog guardrail and the response metrics
// ask how the WORK is going rather than what to do next, and they are read by
// whoever can change how it goes. One spelling for all three, so the answer to
// "who may read this" does not depend on which endpoint you ask.
//
// A POLICY narrowing and not a confinement: all three are already counted under
// the caller's own visibility, so an ungated read discloses no row the reader
// could not open — only a surface they have no use for.

import (
	"context"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// requireLeadTier refuses a reader below a row scope of `team`.
//
// The KIND check first, and it is not decoration: a tier test answers from
// `Permissions`, and a Deal Room buyer is minted carrying none — so it would be
// refused by the accident of an empty struct, and the first constructor to give
// a buyer permissions would hand an external contact the seller's team roster.
// RequireHuman also turns away an agent passport, matching the `human-only`
// these endpoints declare in the contract.
//
// A background pass wanting these figures would need to say so here rather than
// arriving by default on a system principal's row scope.
func requireLeadTier(ctx context.Context) error {
	if err := auth.RequireHuman(ctx); err != nil {
		return err
	}
	actor, ok := principal.Actor(ctx)
	if !ok {
		return apperrors.ErrPermissionDenied
	}
	switch actor.Permissions.RowScope {
	case principal.RowScopeTeam, principal.RowScopeAll:
		return nil
	default:
		return apperrors.ErrPermissionDenied
	}
}
