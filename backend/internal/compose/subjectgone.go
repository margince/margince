// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// unactionableIfGone turns a store's ErrNotFound into events.ErrUnactionable,
// so the bus acks the entry once. Any other error passes through and is
// retried. Call it only where ErrNotFound means no retry can succeed.
func unactionableIfGone(err error, kind string, id ids.UUID) error {
	if errors.Is(err, apperrors.ErrNotFound) {
		return fmt.Errorf("%w: %s %s", events.ErrUnactionable, kind, id)
	}
	return err
}

// unactionableIfDealAbsent is unactionableIfGone for work an archived deal
// must not lose. A deal that still exists, archived or not, keeps the error
// unchanged, so the entry is retried and succeeds once the deal is restored.
func unactionableIfDealAbsent(ctx context.Context, pool *pgxpool.Pool, err error, deal ids.UUID) error {
	if !errors.Is(err, apperrors.ErrNotFound) {
		return err
	}
	var exists bool
	if probeErr := InstallationDB(pool).Tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM deal WHERE id = $1)`, deal).Scan(&exists)
	}); probeErr != nil {
		return errors.Join(err, probeErr)
	}
	if exists {
		return err
	}
	return unactionableIfGone(err, "deal", deal)
}
