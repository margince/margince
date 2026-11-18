// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package licensecheck

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
	"time"

	"github.com/tetratelabs/wazero/sys"

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

// childEnv names the test a child process runs in place of its parent.
const childEnv = "LICENSECHECK_HANG_CHILD"

// hangLimit caps how long the parent waits for a child running a hung module.
// It is a cap on a regression, not a timing the result depends on.
const hangLimit = 20 * time.Second

// inChild runs the calling test in a child process and reports whether this
// process is that child. A wasm loop without the stop holds its thread past Go's
// preemption, so only a separate process can fail it fast.
func inChild(t *testing.T) bool {
	t.Helper()
	if os.Getenv(childEnv) == t.Name() {
		return true
	}
	// The child is this test binary; it reads only the module the build embeds.
	if err := gatekit.DeclareInputs(os.Getenv, "module"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), hangLimit)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^"+t.Name()+"$", "-test.count=1", "-test.v")
	cmd.Env = append(os.Environ(), childEnv+"="+t.Name())
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("a module that never answers ran past its deadline: the child was still running after %s", hangLimit)
	}
	if err != nil {
		t.Fatalf("child run failed: %v\n%s", err, out)
	}
	// A child that matched no test also exits 0, so the pass line is required.
	if !strings.Contains(string(out), "--- PASS: "+t.Name()) {
		t.Fatalf("the child never ran %s:\n%s", t.Name(), out)
	}
	return false
}

func TestAHungModuleStopsWhenItsDeadlinePasses(t *testing.T) {
	t.Parallel()
	if !inChild(t) {
		return
	}
	_, err := check(expired(t), hangingModule, issuer, product, generation, "any-token")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v, want the deadline named so a caller can tell a timeout apart", err)
	}
	if errors.Is(err, ErrVerdict) {
		t.Errorf("a stopped run reads as a refused license: %v", err)
	}
}

// A re-check whose budget ends while the module hangs keeps the posture, and
// reports the stop rather than a refusal.
func TestARecheckThatRunsOutOfBudgetKeepsThePosture(t *testing.T) {
	t.Parallel()
	if !inChild(t) {
		return
	}
	source := &tokens{}
	w, err := NewWatcher(t.Context(), source.source, fixedClock(checkedAt), slog.New(slog.DiscardHandler), runtimeenv.Production)
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}
	w.budget, w.module = 0, hangingModule
	source.token = "any-token"
	err = w.Recheck(t.Context())
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Recheck err = %v, want the expired budget reported", err)
	}
	if got := w.Posture().State; got != StateAbsent {
		t.Errorf("posture = %q after a stopped re-check, want %q kept", got, StateAbsent)
	}
}

// A module that exits with a refusal keeps its verdict even when the context
// has ended by the time the exit is read.
func TestARefusalPastTheDeadlineStaysAVerdict(t *testing.T) {
	t.Parallel()
	if stoppedByContext(expired(t), sys.NewExitError(1)) {
		t.Error("a module that exited by itself past the deadline reads as stopped, so its verdict is lost")
	}
	if !stoppedByContext(expired(t), sys.NewExitError(sys.ExitCodeDeadlineExceeded)) {
		t.Error("wazero's own stop on the deadline does not read as stopped")
	}
	if stoppedByContext(t.Context(), sys.NewExitError(sys.ExitCodeDeadlineExceeded)) {
		t.Error("a module exiting with the stop code while its context is live reads as stopped")
	}
}
