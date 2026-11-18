// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package licensecheck

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/runtimeenv"
)

// hangingModule is a WebAssembly module whose _start loops forever: a validator
// that never answers.
var hangingModule = []byte{
	0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00, // "\0asm", version 1
	0x01, 0x04, 0x01, 0x60, 0x00, 0x00, // type section: func () -> ()
	0x03, 0x02, 0x01, 0x00, // function section: one function of type 0
	0x07, 0x0a, 0x01, 0x06, '_', 's', 't', 'a', 'r', 't', 0x00, 0x00, // export _start
	0x0a, 0x09, 0x01, 0x07, 0x00, 0x03, 0x40, 0x0c, 0x00, 0x0b, 0x0b, // body: loop { br 0 }
}

// expired is a context whose deadline has already passed, so no test waits on
// a clock for the budget to run out.
func expired(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithDeadline(t.Context(), time.Unix(0, 0))
	t.Cleanup(cancel)
	return ctx
}

// Without the stop this call never returns. The loop holds its thread without
// a preemption point, so the whole test binary hangs until go test's -timeout.
func TestAHungModuleStopsWhenItsDeadlinePasses(t *testing.T) {
	t.Parallel()
	_, err := check(expired(t), hangingModule, issuer, product, generation, "any-token")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v, want the deadline named so a caller can tell a timeout apart", err)
	}
	if errors.Is(err, ErrVerdict) {
		t.Errorf("a stopped run reads as a refused license: %v", err)
	}
}

// The budget expiring mid-run must leave the posture alone. With the stop read
// as a verdict, this token would store "rejected" over the posture.
func TestARecheckThatRunsOutOfBudgetKeepsThePosture(t *testing.T) {
	t.Parallel()
	source := &tokens{}
	w, err := NewWatcher(t.Context(), source.source, fixedClock(checkedAt), slog.New(slog.DiscardHandler), runtimeenv.Production)
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}
	w.budget = 0
	source.token = "not-a-license"
	err = w.Recheck(t.Context())
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Recheck err = %v, want the expired budget reported", err)
	}
	if got := w.Posture().State; got != StateAbsent {
		t.Errorf("posture = %q after a stopped re-check, want %q kept", got, StateAbsent)
	}
}
