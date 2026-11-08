// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package database

import (
	"context"

	"github.com/jackc/pgx/v5"
)

type (
	transactionGuardKey struct{}
	transactionGuard    func(context.Context, pgx.Tx) error
)

// WithTransactionGuard checks execution ownership before any domain work, including snapshot joins.
func WithTransactionGuard(ctx context.Context, guard func(context.Context, pgx.Tx) error) context.Context {
	previous, exists := ctx.Value(transactionGuardKey{}).(transactionGuard)
	return context.WithValue(ctx, transactionGuardKey{}, transactionGuard(func(ctx context.Context, tx pgx.Tx) error {
		if exists {
			if err := previous(ctx, tx); err != nil {
				return err
			}
		}
		return guard(ctx, tx)
	}))
}

func guardedTransaction(ctx context.Context, tx pgx.Tx, fn func(pgx.Tx) error) error {
	if guard, exists := ctx.Value(transactionGuardKey{}).(transactionGuard); exists {
		if err := guard(ctx, tx); err != nil {
			return err
		}
	}
	return fn(tx)
}

// WithoutTransactionGuard lets accounting or compensation finish for work that already committed.
func WithoutTransactionGuard(ctx context.Context) context.Context {
	return context.WithValue(ctx, transactionGuardKey{}, struct{}{})
}
