// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"fmt"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// mayReleaseHeld decides who may move a held message back to scheduled. That
// move sends it, so an agent needs the send scope and a human present, the
// same bar approvals.agentMayDecide sets for approving the held-message card.
func mayReleaseHeld(ctx context.Context) error {
	p, ok := principal.Actor(ctx)
	if !ok {
		return fmt.Errorf("releasing a held message needs a signed-in caller: %w", apperrors.ErrPermissionDenied)
	}
	switch {
	case p.Type == principal.PrincipalHuman:
		return nil
	case p.Type != principal.PrincipalAgent:
		return fmt.Errorf("only a user or their agent may release a held message: %w", apperrors.ErrPermissionDenied)
	case principal.Unattended(ctx):
		return fmt.Errorf("an unattended agent cannot release a held message; a human decides it: %w", apperrors.ErrPermissionDenied)
	case !p.Scopes.Has(principal.ScopeSend):
		return fmt.Errorf("releasing a held message sends it, which needs the send permission: %w", apperrors.ErrPermissionDenied)
	}
	return nil
}
