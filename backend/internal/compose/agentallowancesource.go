// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/agentvolume"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// agentAllowanceSource answers the meter with the allowances an admin saved.
type agentAllowanceSource struct{ pool *pgxpool.Pool }

func (s agentAllowanceSource) Policy(ctx context.Context) (agentvolume.Policy, error) {
	var policy agentvolume.Policy
	err := database.WithWorkspaceTx(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		policy, err = identity.AgentPolicyTx(ctx, tx)
		return err
	})
	if err != nil {
		return agentvolume.Policy{}, fmt.Errorf("compose: reading the agent allowances: %w", err)
	}
	return policy, nil
}

// connectionReleaser releases the counter a passport's calls were charged to.
// The staged step-up names the passport; the meter counts a connection's
// passports together, so the subject is resolved here from the passport row.
type connectionReleaser struct {
	meter *agentvolume.Meter
	pool  *pgxpool.Pool
}

func (r connectionReleaser) Release(
	ctx context.Context, ws, passport ids.UUID, c agentvolume.Counter, bucket int64, length time.Duration,
) (bool, error) {
	var connection ids.UUID
	err := database.WithWorkspaceTx(ctx, r.pool, func(tx pgx.Tx) error {
		var err error
		connection, err = identity.PassportConnectionTx(ctx, tx, passport)
		return err
	})
	if err != nil {
		return false, fmt.Errorf("compose: finding the connection of passport %s: %w", passport, err)
	}
	return r.meter.Release(ctx, ws, agentvolume.SubjectOf(passport, connection), c, bucket, length)
}
