// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose/briefs"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/notices"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type morningDigest struct {
	lines  digestLines
	window notices.Window
}

func notificationWindow(ctx context.Context, tx pgx.Tx, now time.Time) (notices.Window, error) {
	day, _, err := briefs.LocalDayAt(ctx, tx, now)
	if err != nil {
		return notices.Window{}, err
	}
	return notices.Window{Day: day, Morning: now.UTC()}, nil
}

func prepareRoutineDigest(ctx context.Context, pool *pgxpool.Pool, user ids.UUID, now time.Time, settings identity.Delivery) (morningDigest, error) {
	var part morningDigest
	if settings.CombineRoutine == nil || !*settings.CombineRoutine {
		return part, nil
	}
	window, allowed, err := routineWindowFor(ctx, identity.NewService(pool), now, settings)
	if err != nil || !allowed {
		return part, err
	}
	err = database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		var err error
		part.window, err = notificationWindow(ctx, tx, window.Open)
		return err
	})
	if err != nil {
		return part, err
	}
	worker := newNotificationDigestWorker(pool, NotificationMailConfig{}, nil)
	worker.now = func() time.Time { return now }
	part.lines, err = worker.readDigestLines(ctx, ids.From[ids.UserKind](user), part.window)
	return part, err
}
