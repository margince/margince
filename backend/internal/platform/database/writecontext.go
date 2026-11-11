// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package database

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// WithWorkspaceWrite joins nested store writes in one READ COMMITTED transaction.
// A row lock can then wait for a writer and read the value that writer committed.
func WithWorkspaceWrite(ctx context.Context, pool *pgxpool.Pool, fn func(context.Context) error) error {
	return withWorkspaceTransaction(ctx, pool, pgx.TxOptions{IsoLevel: pgx.ReadCommitted}, fn)
}

func withWorkspaceTransaction(ctx context.Context, pool *pgxpool.Pool, options pgx.TxOptions, fn func(context.Context) error) error {
	if _, ok := principal.WorkspaceID(ctx); !ok {
		return ErrNoWorkspace
	}
	if _, joined := snapshotOf(ctx); joined {
		return fn(ctx)
	}
	return runTxWith(ctx, pool, options, func(tx pgx.Tx) error {
		return fn(context.WithValue(ctx, snapshotKey{}, tx))
	})
}
