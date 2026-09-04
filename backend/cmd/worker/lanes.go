// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

// What the event lanes leave behind, and how they end. Apart from boot.go
// because starting them and ending them are two concerns and only one of them
// runs at shutdown, under the deferred closes whose ordering join() is what
// makes safe.

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/integrations"
	"github.com/margince/margince/backend/internal/modules/webhooks"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/database"
)

// workerLanes is what the event lanes leave behind for the job runner, which
// schedules against the SAME instances those lanes consume into: one governed
// registry and one brain per role, ONE deliverer FACTORY across both outbound-webhook
// lanes (E10/S-E10.6) — never two that could drift apart — plus the object
// store the deep-read logo write and the retention purge share.
type workerLanes struct {
	// background is the caller's group, held so the lanes started here are
	// counted where the caller waits — it is not this value's to create, and a
	// group made here would be one the caller's join never waits on (#454).
	// It holds every lane goroutine so run() returns only after
	// in-flight handlers finish their ack — the same shape as cmd/api's relay
	// group; a bare goroutine would be killed mid-handler when the relay
	// returns.
	background *sync.WaitGroup
	// laneCtx is what the caller's cancel ends, carried so a lane started LATER
	// than this
	// value — the extension subscriptions, which wait for the runtime binding
	// the job runner makes — still lives and dies with all the others. A second
	// context there would be a second shutdown, and join() would return with a
	// consumer still reading the bus.
	ctx       context.Context //nolint:containedctx // the lanes' lifetime IS this value; join() is the only thing that ends it.
	runner    *compose.RunnerService
	deliverer func(*database.DB) *webhooks.Deliverer
	blob      blobstore.Store
	// providers is the licensed-data-provider adapter registry this boot was
	// configured with (MARGINCE_PROVIDER_SURFE). Nil is a deployment with no
	// provider: the run lanes register nothing and nothing can reach a vendor.
	providers *integrations.Registry
	// logger is carried for join(), which reports a lane that did not stop.
	// The lanes themselves were handed one at construction and do not read
	// this.
	logger *slog.Logger
}

// laneJoinWindow bounds join(). The same window the job drain gets, because it
// is the same promise being kept — nothing this process started is still
// running when it closes what that thing writes through.
const laneJoinWindow = 30 * time.Second

// join ends the lanes and waits for the handler each is in. It is what makes the
// bus and the pool safe to close: run() defers both closes before the lanes
// start, so LIFO runs them after this returns, never under a live subscriber.
//
// Bounded, and that bound is a real weakening of the sentence above — on
// overrun this returns with a lane still reading, and the closes follow. It is
// the better failure anyway. Unbounded, a handler that ignores cancellation
// hangs the process silently, and on the boot-failure path that turns an
// `exit 1` into no exit at all: a supervisor reads a process that never leaves
// as still starting, and the boot error nobody sees was the whole point of
// failing. An overrun that says which promise it broke can be acted on.
// laneLifetime is the lanes' context, the group that counts them, and the one
// call that ends both.
//
// Returned TOGETHER so a caller cannot hold one without the other, and so the
// end is a value it can defer before anything has started. That is the whole of
// #454: the ordering used to be a `defer lanes.join()` written after the call
// that started them — correct, and one edit from silently wrong, with nothing
// in the repository failing when it was.
//
// The caller still has to defer it. What it can no longer do is start a lane
// first, because the group it must be given comes from here.
func laneLifetime(ctx context.Context, logger *slog.Logger) (context.Context, *sync.WaitGroup, func()) {
	laneCtx, stop := context.WithCancel(ctx)
	background := &sync.WaitGroup{}
	return laneCtx, background, func() { joinLanes(stop, background, logger) }
}

// joinLanes ends every lane and waits for them, bounded by the same window
// join() used.
//
// A FUNCTION over the two values rather than a method on the lanes value,
// because run() defers it before that value exists — which is the whole point:
// a method could only be deferred after the call that produced its receiver,
// and "after" is exactly the ordering #454 showed nothing was holding.
func joinLanes(stop context.CancelFunc, background *sync.WaitGroup, logger *slog.Logger) {
	joinLanesWithin(stop, background, logger, laneJoinWindow)
}

// joinLanesWithin is joinLanes with the window as an argument, so a test can
// watch an overrun happen instead of waiting out the real one.
func joinLanesWithin(stop context.CancelFunc, background *sync.WaitGroup, logger *slog.Logger, window time.Duration) {
	stop()
	stopped := make(chan struct{})
	go func() {
		background.Wait()
		close(stopped)
	}()
	select {
	case <-stopped:
	case <-time.After(window):
		// ERROR, and the sentence the method carried before this became a
		// function: an overrun means the process closes the bus and the pool
		// with a subscriber still reading them, so what it does next fails
		// against a closed client. Downgrading that to a warning would make
		// the loudest thing this shutdown can say quieter than the routine
		// lines around it.
		logger.Error("a lane did not stop inside its window; this process closes the bus and the pool with a subscriber still reading them, so whatever it does next fails against a closed client",
			"window", window)
	}
}
