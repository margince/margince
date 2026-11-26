// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package forwardexpiry destroys the held forwards whose window closed.
package forwardexpiry

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
)

// Destroyer is the one purge executor, which compose's capture purger owns. A
// nil purger is a role with no object store, and its method does nothing.
type Destroyer interface {
	DestroyExpiredForwards(ctx context.Context, subject capture.PurgeSubject) error
}

// Run selects the held forwards past their window and hands them to the
// executor, which is what every selected purge goes through.
func Run(ctx context.Context, pool *pgxpool.Pool, purge Destroyer) error {
	var subject capture.PurgeSubject
	if err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) (err error) {
		subject, err = capture.SelectExpiredForwardsTx(ctx, tx, time.Now())
		return err
	}); err != nil {
		return err
	}
	return purge.DestroyExpiredForwards(ctx, subject)
}
