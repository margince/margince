// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package inquiries

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/webforms"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/backoff"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

// WebFormFence holds one web form's handling under its work lease, drawing a
// session slot from the one budget of the form's pool.
type WebFormFence struct {
	db       *database.DB
	budgets  *webFormBudgets
	capacity func() int
	bound    func(context.Context, chan struct{}) context.Context
}

// An intake holds the form's lease for well under a second. A caller that
// waits this long lands behind it instead of refusing a visitor. Each try is a
// pool checkout and a lock query, so the waits between tries grow.
const (
	busyPatience  = 5 * time.Second
	busyFirstWait = 25 * time.Millisecond
	busyMaxWait   = 500 * time.Millisecond
)

// busyAttempts counts the tries whose unjittered waits fit in busyPatience.
func busyAttempts() int {
	var waited time.Duration
	for attempts := 1; ; attempts++ {
		wait := min(busyFirstWait<<(attempts-1), busyMaxWait)
		if waited+wait > busyPatience {
			return attempts
		}
		waited += wait
	}
}

// NewWebFormFence draws on the shared budget of db's pool, so every caller over
// that pool keeps within MaxConns/4. bound puts the slots on the work's context.
func NewWebFormFence(db *database.DB, bound func(context.Context, chan struct{}) context.Context) *WebFormFence {
	return &WebFormFence{db: db, budgets: &sharedWebFormBudgets, bound: bound, capacity: func() int {
		return max(1, int(db.Pool().Config().MaxConns)/4)
	}}
}

// webFormBudgets holds one slot channel per pool. A channel is sized on first
// use, because a composition built over an unconnected pool has no
// configuration to read until a request arrives.
type webFormBudgets struct {
	mu     sync.Mutex
	byPool map[*pgxpool.Pool]chan struct{}
}

var sharedWebFormBudgets webFormBudgets

func (f *WebFormFence) sessionSlots() chan struct{} {
	b := f.budgets
	b.mu.Lock()
	defer b.mu.Unlock()
	pool := f.db.Pool()
	if slots, ok := b.byPool[pool]; ok {
		return slots
	}
	if b.byPool == nil {
		b.byPool = map[*pgxpool.Pool]chan struct{}{}
	}
	slots := make(chan struct{}, f.capacity())
	b.byPool[pool] = slots
	return slots
}

// Within runs work while this caller holds the form's lease. A lease or session
// slot held elsewhere past busyPatience answers webforms.ErrHandlingBusy.
func (f *WebFormFence) Within(ctx context.Context, id ids.UUID, work func(context.Context) error) error {
	// Unwired compositions must reach the store's normal access and database refusals.
	if f.db.Pool() == nil {
		return work(ctx)
	}
	if owned, err := f.db.WorkLeaseOwned(ctx, provenance.WebFormSource, id); err != nil {
		return err
	} else if owned {
		return work(ctx)
	}
	pause := func(ctx context.Context, prior int) error {
		return backoff.Sleep(ctx, backoff.Jittered(prior, busyFirstWait, busyMaxWait))
	}
	err := awaitLease(ctx, busyAttempts(), pause, func(ran *bool) error {
		held, err := f.db.WithWorkLease(f.bound(ctx, f.sessionSlots()), provenance.WebFormSource, id, slog.Default(),
			func(leased context.Context) error {
				*ran = true
				return work(leased)
			})
		if errors.Is(err, database.ErrSessionBudgetBusy) || (!held && err == nil) {
			return webforms.ErrHandlingBusy
		}
		return err
	})
	if errors.Is(err, webforms.ErrHandlingBusy) {
		slog.WarnContext(ctx, "web form handling stayed busy; the caller was told to retry", "form_id", id)
	}
	return err
}

// awaitLease repeats try while the lease was busy before work ran, telling
// pause how many tries were busy before it. A busy answer from inside work is
// the work's own, and a repeat could write twice.
func awaitLease(ctx context.Context, attempts int, pause func(ctx context.Context, prior int) error, try func(ran *bool) error) error {
	for attempt := 1; ; attempt++ {
		ran := false
		err := try(&ran)
		if ran || attempt >= attempts || !errors.Is(err, webforms.ErrHandlingBusy) {
			return err
		}
		if err := pause(ctx, attempt-1); err != nil {
			return err
		}
	}
}
