// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

import (
	"fmt"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// RequireToolScope shares passport admission between direct calls and approvals.
func RequireToolScope(p principal.Principal, spec mcp.ToolSpec) error {
	if p.Type == principal.PrincipalAgent && !spec.SelfDescribing && !p.Scopes.Has(spec.RequiredScope) {
		return &ScopeRequiredError{Tool: spec.Name, Scope: spec.RequiredScope}
	}
	return nil
}

// RequireToolSeat keeps approvals subject to the direct-call seat requirement.
func RequireToolSeat(p principal.Principal, spec mcp.ToolSpec) error {
	if p.Type == principal.PrincipalAgent && spec.RequiredScope != principal.ScopeRead && !p.SeatType.CanMutate() {
		return fmt.Errorf("gate: %s needs a full seat; %s acts for a read seat: %w", spec.Name, p.ID, apperrors.ErrSeatTierInsufficient)
	}
	return nil
}
