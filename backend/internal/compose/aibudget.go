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
	"github.com/margince/margince/backend/internal/platform/settings"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// seatBudget derives the pool live: seat changes move the budget at
// the next model call, no restart. The count runs against app_user,
// which carries no workspace column at all (ADR-0091 §8 phase D) — a
// single-company installation has one workspace to charge, so
// counting every full seat on the installation IS the tenant's count.
type seatBudget struct {
	pool *pgxpool.Pool
}

// NewSeatBudget is the production BudgetPolicy.
func NewSeatBudget(pool *pgxpool.Pool) ai.BudgetPolicy { return seatBudget{pool: pool} }

func (b seatBudget) MonthlyTokenBudget(ctx context.Context, workspaceID ids.WorkspaceID) (int64, error) {
	var monthly int64
	err := database.WithWorkspaceTx(principal.WithWorkspaceID(ctx, workspaceID.UUID), b.pool, func(tx pgx.Tx) error {
		config, err := settings.ApplyTx(ctx, tx, ai.BudgetSettings)
		if err != nil {
			return err
		}
		users, err := budgetFullUsers(ctx, tx)
		if err != nil {
			return err
		}
		monthly, err = config.MonthlyTokens(users)
		return err
	})
	return monthly, err
}

func budgetFullUsers(ctx context.Context, tx pgx.Tx) (int64, error) {
	var users int64
	err := tx.QueryRow(ctx, `SELECT count(*) FROM app_user
 WHERE seat_type = 'full' AND `+identity.LiveMemberSQL("")+` AND NOT is_agent`).Scan(&users)
	return users, err
}
