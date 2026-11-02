// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package instrument holds the hooks extension units returned from their
// Instrumentation.Start, for the life of the process, and is the one place the
// core's choke points call them through. Every call is guarded: a hook that
// panics is logged and skipped, so instrumentation can never take the product
// down with it.
//
// It is set once at boot (Install) and read everywhere after, the same
// lifetime as the process logger.
package instrument

import (
	"context"
	"log/slog"
	"net/http"
	"runtime/debug"
	"slices"
	"sync"
	"sync/atomic"

	"github.com/margince/margince/backend/pkg/extension"
)

// Started is one unit's hooks, as its Start returned them.
type Started struct {
	Unit  string
	Hooks extension.Instruments
}

type installed struct {
	units []Started
	log   *slog.Logger
}

// current is the process-wide holder. Install sets it at boot and every
// request and job reads it, so it is an atomic pointer rather than a mutex.
var current atomic.Pointer[installed]

// Install records the started units, in registry order. A role calls it once,
// at boot, before it builds its logger or serves.
func Install(started []Started, log *slog.Logger) {
	current.Store(&installed{units: slices.Clone(started), log: log})
}

// Reset forgets every installed unit. Tests only.
func Reset() { current.Store(nil) }

func installedUnits() ([]Started, *slog.Logger) {
	in := current.Load()
	if in == nil {
		return nil, nil
	}
	return in.units, in.log
}

// safely runs fn and reports whether it panicked. The panic is logged against
// the unit and hook that raised it, and does not propagate.
func safely(log *slog.Logger, unit, hook string, fn func()) (panicked bool) {
	return guard(log, unit, hook, nil, fn)
}

// guard is safely with one exception: when quiet reports true, a panic is
// contained but not logged, because it is the work's own panic passing through
// the hook on its way to the caller, not a fault of the hook.
func guard(log *slog.Logger, unit, hook string, quiet func() bool, fn func()) (panicked bool) {
	defer func() {
		if rec := recover(); rec != nil {
			panicked = true
			if log != nil && (quiet == nil || !quiet()) {
				log.Error("instrumentation hook panicked",
					"unit", unit, "hook", hook, "panic", rec, "stack", string(debug.Stack()))
			}
		}
	}()
	fn()
	return false
}

// LogHandler wraps h in every unit's LogHandler, the first unit outermost.
func LogHandler(h slog.Handler) slog.Handler {
	units, log := installedUnits()
	for _, u := range slices.Backward(units) {
		if u.Hooks.LogHandler == nil {
			continue
		}
		safely(log, u.Unit, "LogHandler", func() {
			if wrapped := u.Hooks.LogHandler(h); wrapped != nil {
				h = wrapped
			}
		})
	}
	return h
}

// HTTP wraps h in every unit's HTTP hook, the first unit outermost.
func HTTP(h http.Handler) http.Handler {
	units, log := installedUnits()
	for _, u := range slices.Backward(units) {
		if u.Hooks.HTTP == nil {
			continue
		}
		safely(log, u.Unit, "HTTP", func() {
			if wrapped := u.Hooks.HTTP(h); wrapped != nil {
				h = wrapped
			}
		})
	}
	return h
}

// Work runs run inside every unit's Work hook, the first unit outermost. run
// executes exactly once whatever the hooks do: a hook that panics before
// calling it, or returns without calling it, still has it run.
func Work(ctx context.Context, w extension.Work, run func(context.Context) error) error {
	units, log := installedUnits()
	call := run
	for _, u := range slices.Backward(units) {
		call = wrapWork(log, u, w, call)
	}
	return call(ctx)
}

func wrapWork(log *slog.Logger, u Started, w extension.Work, inner func(context.Context) error) func(context.Context) error {
	if u.Hooks.Work == nil {
		return inner
	}
	return func(ctx context.Context) error {
		var (
			once    sync.Once
			mu      sync.Mutex // guards the fields below; the hook may call run from another goroutine
			ran     bool
			runErr  error
			workPan any
			workBad bool
		)
		run := func(ctx context.Context) (err error) {
			first := false
			once.Do(func() {
				first = true
				mu.Lock()
				ran = true
				mu.Unlock()
				defer func() {
					if rec := recover(); rec != nil {
						mu.Lock()
						workPan, workBad = rec, true
						mu.Unlock()
						// Let the hook see the work's panic unwind through it.
						panic(rec)
					}
				}()
				e := inner(ctx)
				mu.Lock()
				runErr = e
				mu.Unlock()
			})
			if !first {
				return nil
			}
			mu.Lock()
			defer mu.Unlock()
			return runErr
		}
		var hookErr error
		panicked := guard(log, u.Unit, "Work", func() bool {
			mu.Lock()
			defer mu.Unlock()
			return workBad
		}, func() {
			hookErr = u.Hooks.Work(ctx, w, run)
		})
		mu.Lock()
		didRun, err, bad, pan := ran, runErr, workBad, workPan
		mu.Unlock()
		switch {
		case bad:
			// The work panicked: it reaches the caller unchanged, whatever the hook did.
			panic(pan)
		case !didRun:
			// The hook panicked or returned without calling run: the work still runs.
			return inner(ctx)
		case panicked:
			// The hook panicked after the work ran: its outcome is the work's.
			return err
		default:
			return hookErr
		}
	}
}

// Panic tells every unit about a recovered panic. It never re-panics.
func Panic(ctx context.Context, value any, stack []byte) {
	units, log := installedUnits()
	for _, u := range units {
		if u.Hooks.Panic == nil {
			continue
		}
		safely(log, u.Unit, "Panic", func() {
			u.Hooks.Panic(ctx, value, stack)
		})
	}
}

// Shutdown flushes every unit, logging (not returning) a failure.
func Shutdown(ctx context.Context) {
	units, log := installedUnits()
	for _, u := range units {
		if u.Hooks.Shutdown == nil {
			continue
		}
		safely(log, u.Unit, "Shutdown", func() {
			if err := u.Hooks.Shutdown(ctx); err != nil && log != nil {
				log.Warn("instrumentation shutdown failed", "unit", u.Unit, "err", err)
			}
		})
	}
}
