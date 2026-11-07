// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
)

func watchBackfillOwnership(ctx context.Context, ticks <-chan time.Time, check func() error, cancel context.CancelCauseFunc) {
	reported := false
	for {
		select {
		case <-ctx.Done():
			return
		case _, open := <-ticks:
			if !open {
				return
			}
			err := check()
			if errors.Is(err, apperrors.ErrConflict) {
				cancel(err)
				return
			}
			if err != nil {
				if !reported {
					slog.WarnContext(ctx, "mail import ownership probe failed; checking again without ending the page", "err", err)
					reported = true
				}
				continue
			}
			reported = false
		}
	}
}

// The transaction fence provides exclusion; polling also stops provider I/O after cancellation or lease loss.
func (r *Registry) monitorBackfill(ctx context.Context) (context.Context, func()) {
	ctx, cancel := context.WithCancelCause(ctx)
	ticker := time.NewTicker(r.ownershipWatch)
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		watchBackfillOwnership(ctx, ticker.C, func() error {
			probe, stop := context.WithTimeout(ctx, 2*time.Second)
			defer stop()
			if execution, ok := ctx.Value(backfillExecutionKeyType{}).(backfillExecution); ok {
				if err := execution.lease.Ping(probe); err != nil {
					return fmt.Errorf("capture: execution connection lost: %w: %v", apperrors.ErrConflict, err)
				}
			}
			return r.db.Tx(probe, func(pgx.Tx) error { return nil })
		}, cancel)
	}()
	return ctx, func() { cancel(nil); ticker.Stop(); <-stopped }
}
