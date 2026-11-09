// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

func TestOwnershipLossCancelsProviderWorkAtTheNextProbe(t *testing.T) {
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	lost := apperrors.ErrConflict
	ticks := make(chan time.Time)
	stopped := make(chan struct{})
	go func() { defer close(stopped); watchBackfillOwnership(ctx, ticks, func() error { return lost }, cancel) }()
	ticks <- clocktest.Now(t)
	<-stopped
	if !errors.Is(context.Cause(ctx), lost) {
		t.Fatalf("provider cancellation cause: %v", context.Cause(ctx))
	}
}

func TestAnEndedPageStopsOwnershipProbes(t *testing.T) {
	ctx, cancel := context.WithCancelCause(context.Background())
	cancel(nil)
	calls := 0
	watchBackfillOwnership(ctx, make(chan time.Time), func() error { calls++; return nil }, cancel)
	if calls != 0 {
		t.Fatalf("%d probes after page ended", calls)
	}
}

func TestATransientProbeErrorDoesNotEndThePageOrItsNextProbe(t *testing.T) {
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	ticks := make(chan time.Time)
	stopped := make(chan struct{})
	checks := 0
	go func() {
		defer close(stopped)
		watchBackfillOwnership(ctx, ticks, func() error {
			checks++
			if checks == 1 {
				return context.DeadlineExceeded
			}
			if checks == 3 {
				return apperrors.ErrConflict
			}
			return nil
		}, cancel)
	}()
	for range 2 {
		select {
		case ticks <- clocktest.Now(t):
		case <-stopped:
			t.Fatal("a transient probe ended the watcher")
		}
	}
	if ctx.Err() != nil {
		t.Fatal("a transient probe cancelled provider work")
	}
	ticks <- clocktest.Now(t)
	<-stopped
	if checks != 3 || !errors.Is(context.Cause(ctx), apperrors.ErrConflict) {
		t.Fatalf("checks=%d cause=%v", checks, context.Cause(ctx))
	}
}
