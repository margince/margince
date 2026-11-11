// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package licensecheck

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"github.com/margince/margince/backend/internal/shared/runtimeenv"
)

// recheckInterval is how often a running process re-resolves its posture. A
// license changes state on a calendar — expiry, then expiry plus grace — so
// daily is as often as the answer can differ, and a re-check costs one
// short-lived wazero instantiation.
const recheckInterval = 24 * time.Hour

// recheckBudget bounds the context of a re-check that no longer follows its
// caller. One token read and one module run take seconds; a minute leaves room
// for a loaded host.
const recheckBudget = time.Minute

// TokenSource reads the installation's license token under the current read's
// context. Reading it afresh lets a renewed token take effect without a restart.
type TokenSource func(context.Context) (string, error)

// Watcher holds this installation's license posture and re-resolves it while the
// process runs, so a license crossing expiry-plus-grace — or one renewed in
// place — takes effect without a restart.
//
// It never ends the process. The boot gate is where a refused license stops an
// installation; a process that is already serving degrades instead, because a
// licensing edge case must not take a customer's CRM offline mid-month with no
// human in the loop. Recheck therefore also contains a panic: the module runs
// through a runtime that panics rather than returning on some faults, and this
// type's whole promise is that nothing about a license kills a serving process.
type Watcher struct {
	source  TokenSource
	now     func() time.Time
	log     *slog.Logger
	env     runtimeenv.Environment
	posture atomic.Pointer[Posture]

	// running is the re-check in flight, guarded by mu. One runs at a time, so
	// an older read can never store over a newer one.
	mu      sync.Mutex
	running *recheckRun
}

// recheckRun is one re-check that later callers join instead of starting
// their own. done closes once err is set.
type recheckRun struct {
	done chan struct{}
	err  error
}

// NewWatcher resolves the posture once and refuses a rejected one, so the
// caller's boot fails before the role serves.
//
// An absent license is a posture here, not a refusal. Whether an installation
// may RUN unlicensed depends on the deployment posture rather than on the
// license, so that decision is the composition root's (compose.EnsureLicense)
// and this package reports what it found.
func NewWatcher(ctx context.Context, source TokenSource, now func() time.Time, log *slog.Logger, env runtimeenv.Environment) (*Watcher, error) {
	token, err := source(ctx)
	if err != nil {
		return nil, err
	}
	// A module that could not run refuses the boot exactly as a refused license
	// does: a validation module this build cannot execute is a packaging fault,
	// and booting through it would read as an unlicensed installation.
	resolved, err := Resolve(ctx, token, now(), env)
	if err != nil {
		return nil, err
	}
	if resolved.State == StateRejected {
		// Where the token CAME from is not named here: this package is handed a
		// token, not a configuration file, and the caller that resolved it is the
		// one that can tell the operator which setting to correct.
		return nil, fmt.Errorf("licensecheck: the license was refused by the bundled validation module (%s): %s",
			ModuleVersion(), resolved.Reason)
	}
	w := &Watcher{source: source, now: now, log: log, env: env}
	w.posture.Store(&resolved)
	return w, nil
}

// Posture answers the most recent resolution. Safe for concurrent readers: the
// pointer is swapped whole, so a reader sees one answer or the next and never a
// mixture.
func (w *Watcher) Posture() Posture { return *w.posture.Load() }

// Recheck resolves once and records the answer, reporting a state that CHANGED.
// A steady state is not logged: an operator reading a year of logs should find
// the day the license lapsed, not three hundred and sixty-five lines saying it
// had not.
//
// Anything that is not a verdict — an unreadable token, a module that failed to
// run, a panic on the way there — leaves the posture it already had in place and
// is reported as itself. None of those is evidence about the license, and
// degrading on one would tell an operator their license had been refused when
// what actually broke was the machinery for asking.
//
// The returned error is that fault, already logged, for a caller that waits.
// A caller arriving while a re-check runs joins it and gets its answer. So
// the daily loop and "Check now" never run the module twice at once.
func (w *Watcher) Recheck(ctx context.Context) error {
	w.mu.Lock()
	if run := w.running; run != nil {
		w.mu.Unlock()
		w.log.DebugContext(ctx, "joining the license re-check already running")
		select {
		case <-run.done:
			return run.err
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	run := &recheckRun{done: make(chan struct{})}
	w.running = run
	w.mu.Unlock()

	// Not the caller's cancellation: callers who joined are waiting on this run.
	detached, cancel := context.WithTimeout(context.WithoutCancel(ctx), recheckBudget)
	run.err = w.recheckOnce(detached)
	cancel()
	w.mu.Lock()
	w.running = nil
	w.mu.Unlock()
	close(run.done)
	return run.err
}

// recheckOnce reads the token and resolves it once; Recheck makes sure only one
// runs at a time.
func (w *Watcher) recheckOnce(ctx context.Context) (err error) {
	defer w.surviveFault(ctx, &err)
	before := w.Posture()
	token, err := w.source(ctx)
	if err != nil {
		w.log.ErrorContext(ctx, "re-reading the license failed; keeping the posture this process last resolved",
			"err", err, "posture", string(before.State))
		return fmt.Errorf("licensecheck: re-reading the license token: %w", err)
	}
	after, err := Resolve(ctx, token, w.now(), w.env)
	if err != nil {
		w.log.ErrorContext(ctx, "re-checking the license failed; keeping the posture this process last resolved",
			"err", err, "posture", string(before.State))
		return fmt.Errorf("licensecheck: re-checking the license: %w", err)
	}
	carryCeiling(before, &after)
	w.posture.Store(&after)
	if after.State == before.State {
		return nil
	}
	// Any transition is worth a warning, including one back to valid: a license
	// that recovered did so because somebody replaced the token, and when it
	// recovered is as operationally useful as when it lapsed.
	w.log.WarnContext(ctx, "license posture changed",
		"from", string(before.State), "to", string(after.State),
		"reason", after.Reason, "module", ModuleVersion())
	return nil
}

// surviveFault turns a panic on the re-check path into a logged fault. The
// posture keeps whatever it last held, which is the honest answer: a panicking
// runtime said nothing about the license. The panic becomes the re-check's
// error, so a caller waiting on the answer is not told it succeeded.
func (w *Watcher) surviveFault(ctx context.Context, err *error) {
	fault := recover()
	if fault == nil {
		return
	}
	w.log.ErrorContext(ctx, "re-checking the license panicked; keeping the posture this process last resolved",
		"panic", fmt.Sprint(fault), "module", ModuleVersion(), "stack", string(debug.Stack()))
	*err = fmt.Errorf("licensecheck: re-checking the license panicked: %v", fault)
}

// RunRecheck re-resolves until ctx is cancelled. It is started by each process
// role that resolved a posture at boot; nothing else drives it.
func (w *Watcher) RunRecheck(ctx context.Context) {
	ticker := time.NewTicker(recheckInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			//craft:ignore swallowed-errors Recheck logs its own fault and keeps the last posture; the loop has nobody to tell
			_ = w.Recheck(ctx)
		}
	}
}
