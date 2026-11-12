// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose/attention"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

// attentionSnapshots binds composed reads to one database snapshot.
type attentionSnapshots struct{ pool *pgxpool.Pool }

func (a attentionSnapshots) InSnapshot(ctx context.Context, read string, fn func(context.Context) error) error {
	return database.WithWorkspaceSnapshot(ctx, a.pool, func(ctx context.Context) error {
		if read == attention.ReadWorklist || read == attention.ReadWorklistDone {
			return database.ReuseGenericPlans(ctx, func(ctx context.Context) error {
				return database.WithWorkspaceTx(ctx, a.pool, func(tx pgx.Tx) error {
					prepared, err := storekit.WithMailSeatIdentitySnapshot(ctx, tx)
					if err != nil {
						return err
					}
					return fn(prepared)
				})
			})
		}
		return fn(ctx)
	})
}

func (a attentionSnapshots) Degradable(
	ctx context.Context, budget time.Duration, fn func(context.Context) error,
) error {
	return database.WithDegradableRead(ctx, budget, fn)
}

func (a attentionSnapshots) Detached(ctx context.Context) context.Context {
	return database.Detached(ctx)
}
