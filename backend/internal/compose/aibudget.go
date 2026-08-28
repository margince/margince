// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The seat-derived AI budget (ai-operational-spec §1.3 / 09 §2.4): a
// workspace's monthly token pool is its FULL seats × 6M base × 2
// safety. Composed here because the policy joins ai (the guardrail)
// to identity's seat table — the ai module only ever sees the
// BudgetPolicy seam.

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const (
	perSeatBaseTokens  = 6_000_000
	budgetSafetyFactor = 2
)

// seatBudget derives the pool live: seat changes move the budget at
// the next model call, no restart. The count runs against app_user,
// which carries no workspace column at all (ADR-0091 §8 phase D) — a
// single-organization installation has one workspace to charge, so
// counting every full seat on the installation IS the tenant's count.
type seatBudget struct {
	pool *pgxpool.Pool
}

// NewSeatBudget is the production BudgetPolicy.
func NewSeatBudget(pool *pgxpool.Pool) ai.BudgetPolicy { return seatBudget{pool: pool} }

func (b seatBudget) MonthlyTokenBudget(ctx context.Context, workspaceID ids.WorkspaceID) (int64, error) {
	var fullSeats int64
	err := database.WithWorkspaceTx(principal.WithWorkspaceID(ctx, workspaceID.UUID), b.pool, func(tx pgx.Tx) error {
		// Every full seat on the installation, which is every full seat there
		// is: ADR-0091 §8 phase D took the tenant column off app_user, and a
		// single-organization installation has one workspace to charge.
		return tx.QueryRow(ctx, `
			SELECT count(*) FROM app_user
			WHERE seat_type = 'full' AND `+identity.LiveMemberSQL("")+`
			  AND NOT is_agent`).Scan(&fullSeats)
	})
	if err != nil {
		return 0, err
	}
	if fullSeats == 0 {
		// A workspace with no live full seat still gets the single-seat
		// floor: onboarding flows call the model before the first seat
		// settles, and zero would hard-refuse them.
		fullSeats = 1
	}
	return fullSeats * perSeatBaseTokens * budgetSafetyFactor, nil
}
