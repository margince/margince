// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// approvalEffectActor is the agent that will spend a staged call, authenticated
// by the query its own calls go through. A revoked passport or a retired
// connection resolves to nobody, as the agent's retry would.
func approvalEffectActor(db *database.DB) approvals.EffectActor {
	directory := identity.NewService(db.Pool())
	return func(ctx context.Context, tx pgx.Tx, by approvals.StagedBy) (principal.Principal, error) {
		if by.OnBehalfOf.IsZero() {
			return principal.Principal{}, apperrors.ErrPermissionDenied
		}
		agent, err := directory.AgentForStagedCallTx(ctx, tx, by.OnBehalfOf, ids.From[ids.PassportKind](by.PassportID), by.ConnectionID)
		if err != nil {
			return principal.Principal{}, err
		}
		return agent.Principal(), nil
	}
}
