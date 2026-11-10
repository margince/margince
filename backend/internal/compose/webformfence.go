// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"log/slog"

	"github.com/margince/margince/backend/internal/modules/webforms"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

type webFormFence struct {
	db    *database.DB
	slots chan struct{}
}

func newWebFormFence(db *database.DB) *webFormFence {
	capacity := 1
	if pool := db.Pool(); pool != nil {
		capacity = max(1, int(pool.Config().MaxConns)/4)
	}
	return &webFormFence{db: db, slots: make(chan struct{}, capacity)}
}

func (f *webFormFence) Within(ctx context.Context, id ids.UUID, work func(context.Context) error) error {
	// Unwired compositions must reach the store's normal access and database refusals.
	if f.db.Pool() == nil {
		return work(ctx)
	}

	if owned, err := f.db.WorkLeaseOwned(ctx, provenance.WebFormSource, id); err != nil {
		return err
	} else if owned {
		return work(ctx)
	}
	if _, bounded := ctx.Value(sessionBudgetKey{}).(chan struct{}); !bounded {
		ctx = context.WithValue(ctx, sessionBudgetKey{}, f.slots)
	}
	ctx = database.WithSessionBudget(ctx, takeSessionSlot)
	held, err := f.db.WithWorkLease(ctx, provenance.WebFormSource, id, slog.Default(), work)
	if errors.Is(err, database.ErrSessionBudgetBusy) || (!held && err == nil) {
		return webforms.ErrHandlingBusy
	}
	return err
}
