// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// approvalEffectActor is the agent that staged a proposal, on its human's live
// grants, teams and seat, which is what bounds every row the agent reads.
// Passport scopes are left out: they choose the tools an agent may call, and
// the preview and the context pin only read.
func approvalEffectActor(ctx context.Context, tx pgx.Tx, by approvals.StagedBy) (principal.Principal, error) {
	if by.OnBehalfOf.IsZero() {
		return principal.Principal{}, apperrors.ErrPermissionDenied
	}
	members, err := identity.MemberPrincipalsTx(ctx, tx, []ids.UUID{by.OnBehalfOf})
	if err != nil {
		return principal.Principal{}, err
	}
	actor, live := members[by.OnBehalfOf]
	if !live {
		return principal.Principal{}, apperrors.ErrNotFound
	}
	actor.Type, actor.ID = principal.PrincipalAgent, by.ProposedBy
	actor.OnBehalfOf, actor.PassportID, actor.ConnectionID = by.OnBehalfOf, by.PassportID, by.ConnectionID
	return actor, nil
}
