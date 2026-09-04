// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// The tier that admits the three READINGS ABOUT the queue, as against the queue
// itself.
//
// `/worklist` assembles one person's day and every seat gets their own. The team
// board, the hidden-backlog guardrail and the response metrics are a different
// kind of question: they ask how the WORK is going rather than what to do next,
// and they are read by whoever can change how it goes — the horizon that is set
// wrong, the rep marking every hard reply not_sales, the fortnight nobody
// answered in. A rep working their queue to the bottom is not the person who
// acts on any of those.
//
// One spelling, because two of the three used to have none. The board refused
// below `team` in its own body while the other two checked nothing, so the
// product's answer to "who may read this" depended on which of three endpoints
// you asked — and the client hid all three behind the same tier, which made the
// gap invisible from the browser.
//
// This is a POLICY narrowing and not a confinement. All three are already
// counted under the caller's own visibility, so an ungated read discloses no
// row the reader could not open; what it discloses is a surface they have no
// use for and no way to act on.

import (
	"context"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// requireLeadTier refuses a reader below a row scope of `team`.
func requireLeadTier(ctx context.Context) error {
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
