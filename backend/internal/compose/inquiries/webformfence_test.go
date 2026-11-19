// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package inquiries

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/webforms"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestWebFormFencesOverOnePoolShareOneSessionBudget(t *testing.T) {
	pool := &pgxpool.Pool{}
	over := func(p *pgxpool.Pool) *database.DB { return database.BindTo(p, ids.WorkspaceID{}) }
	intakeFence, routingFence := NewWebFormFence(over(pool), nil), NewWebFormFence(over(pool), nil)
	if intakeFence.budgets != routingFence.budgets {
		t.Fatal("two web form fences draw on two budget registries")
	}

	budgets := &webFormBudgets{}
	fence := func(p *pgxpool.Pool) *WebFormFence {
		return &WebFormFence{db: over(p), budgets: budgets, capacity: func() int { return 1 }}
	}
	intake, routing := fence(pool), fence(pool)
	intake.sessionSlots() <- struct{}{}
	select {
	case routing.sessionSlots() <- struct{}{}:
		t.Fatal("a second caller over the same pool took a slot past the pool's bound")
	default:
	}
	select {
	case fence(&pgxpool.Pool{}).sessionSlots() <- struct{}{}:
	default:
		t.Fatal("a fence over another pool shares this pool's budget")
	}
}

func TestAFenceWaitsOutABusyLeaseBeforeItRefuses(t *testing.T) {
	var priors []int
	noPause := func(_ context.Context, prior int) error {
		priors = append(priors, prior)
		return nil
	}
	busyThen := func(busy int, tries *int) func(*bool) error {
		return func(ran *bool) error {
			*tries++
			if *tries <= busy {
				return webforms.ErrHandlingBusy
			}
			*ran = true
			return nil
		}
	}

	tries := 0
	if err := awaitLease(t.Context(), 5, noPause, busyThen(2, &tries)); err != nil || tries != 3 {
		t.Errorf("a lease freed on the third try: err=%v after %d tries, want the work run", err, tries)
	}
	if !slices.Equal(priors, []int{0, 1}) {
		t.Errorf("the waits were told %v busy tries before them, want [0 1] so each wait grows", priors)
	}
	tries = 0
	if err := awaitLease(t.Context(), 3, noPause, busyThen(10, &tries)); !errors.Is(err, webforms.ErrHandlingBusy) || tries != 3 {
		t.Errorf("a lease held throughout: err=%v after %d tries, want busy after 3", err, tries)
	}

	tries = 0
	fromWork := func(ran *bool) error {
		tries++
		*ran = true
		return webforms.ErrHandlingBusy
	}
	if err := awaitLease(t.Context(), 5, noPause, fromWork); !errors.Is(err, webforms.ErrHandlingBusy) || tries != 1 {
		t.Errorf("work that answered busy itself was tried %d times, want once: a repeat could write twice", tries)
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	tries = 0
	pause := func(ctx context.Context, _ int) error { return ctx.Err() }
	if err := awaitLease(ctx, 5, pause, busyThen(10, &tries)); !errors.Is(err, context.Canceled) || tries != 1 {
		t.Errorf("a caller that gave up: err=%v after %d tries, want its own cancellation", err, tries)
	}
}

func TestAFenceTriesABusyLeaseFarLessOftenThanEveryPollWithinItsPatience(t *testing.T) {
	var waited time.Duration
	attempts := busyAttempts()
	for prior := range attempts - 1 {
		waited += min(busyFirstWait<<prior, busyMaxWait)
	}
	if waited > busyPatience || waited+busyMaxWait <= busyPatience {
		t.Errorf("%d tries wait %v in all, want the last wait to end within %v of the patience %v", attempts, waited, busyMaxWait, busyPatience)
	}
	if attempts > 20 {
		t.Errorf("a busy lease is tried %d times in %v, want at most 20 lock queries per waiting caller", attempts, busyPatience)
	}
}
