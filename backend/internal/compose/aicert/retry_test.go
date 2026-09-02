// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert

// The run-level re-drive: what happens to a task when the router comes back
// having failed on every bound rung. Every test here is offline — the fake
// provider's own scripted error stands in for the transient fault — and the
// backoff is taken through the sleepFunc seam, so nothing waits for real time.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/ai"
)

// ladderRungs is how many scripted client outcomes ONE exhausted ladder walk
// consumes for the task these tests certify. Derived from the contract rather
// than written as a number: ladderForTask binds the candidate to every tier, so
// a walk calls the client once per rung of the task's own ladder, and a ladder
// that grows would otherwise leave these tests scripting a partial walk and
// quietly proving something else.
func ladderRungs(t *testing.T) int {
	t.Helper()
	n := len(ai.TaskLadder(ai.TaskSummarize))
	if n == 0 {
		t.Fatalf("task %s has no ladder — these tests script one walk of it", ai.TaskSummarize)
	}
	return n
}

// failedWalk is the scripted outcomes of one whole ladder walk that reaches
// nobody: every rung refuses with err.
func failedWalk(t *testing.T, err error) []ai.FakeStep {
	t.Helper()
	steps := make([]ai.FakeStep, 0, ladderRungs(t))
	for range ladderRungs(t) {
		steps = append(steps, ai.FakeStep{Err: err})
	}
	return steps
}

// recordSleeps swaps the retry's delay seam for one that records what it was
// asked to wait and returns immediately.
func recordSleeps(t *testing.T) *[]time.Duration {
	t.Helper()
	var waited []time.Duration
	prev := sleepFunc
	sleepFunc = func(_ context.Context, d time.Duration) error {
		waited = append(waited, d)
		return nil
	}
	t.Cleanup(func() { sleepFunc = prev })
	return &waited
}

// errDroppedConnection is the shape of the fault this whole mechanism exists
// for: the connection died before the model was reached, so the same call a
// moment later is free to succeed.
var errDroppedConnection = errors.New("http2: client connection lost")

func TestCertifyTaskRedrivesARunAfterEveryBoundTierFailed(t *testing.T) {
	waited := recordSleeps(t)
	// One whole ladder walk fails, then every later call answers. Without the
	// re-drive this is a task with no record at all.
	candidate := ai.NewFakeClient().
		ScriptSteps(failedWalk(t, errDroppedConnection)...).
		Script(containsWidget, containsWidget, containsWidget)
	judge := ai.NewFakeClient().Script(scoreJSON(90), scoreJSON(90), scoreJSON(90))

	rec, err := certifyTask(wsContext(t), ai.TaskSummarize, []Scenario{testScenario("basic", wideBands)}, testCensus(t),
		ai.ProviderConfig{Provider: ai.ProviderFake, Model: "candidate"}, ai.ProviderConfig{Provider: ai.ProviderFake, Model: "judge"},
		ai.ProfileEUHosted, 3, quietLogger(), &certifyHooks{
			candidateOpts: []ai.LocalOption{ai.WithFakeClient(candidate)},
			judgeOpts:     []ai.LocalOption{ai.WithFakeClient(judge)},
		})
	if err != nil {
		t.Fatalf("a dropped connection on one run must cost that run, not the task: %v", err)
	}
	if rec.Runs != 3 || rec.Reliability != 1 {
		t.Fatalf("runs=%d reliability=%v, want 3 and 1 — the re-driven run must count exactly once", rec.Runs, rec.Reliability)
	}
	if len(*waited) != 1 || (*waited)[0] != runRetryBackoff[0] {
		t.Fatalf("waited %v, want exactly one wait of %v before the second attempt", *waited, runRetryBackoff[0])
	}
}

func TestCertifyTaskDoesNotRedriveAnExhaustedAccount(t *testing.T) {
	waited := recordSleeps(t)
	// Every rung refuses for the one reason another attempt cannot get past.
	candidate := ai.NewFakeClient().ScriptSteps(failedWalk(t, fmt.Errorf("provider said no: %w", ai.ErrProviderQuota))...)
	judge := ai.NewFakeClient().Script(scoreJSON(90))

	_, err := certifyTask(wsContext(t), ai.TaskSummarize, []Scenario{testScenario("basic", wideBands)}, testCensus(t),
		ai.ProviderConfig{Provider: ai.ProviderFake, Model: "candidate"}, ai.ProviderConfig{Provider: ai.ProviderFake, Model: "judge"},
		ai.ProfileEUHosted, 3, quietLogger(), &certifyHooks{
			candidateOpts: []ai.LocalOption{ai.WithFakeClient(candidate)},
			judgeOpts:     []ai.LocalOption{ai.WithFakeClient(judge)},
		})
	if err == nil {
		t.Fatal("an exhausted provider account must fail the task, not be retried into one")
	}
	if len(*waited) != 0 {
		t.Fatalf("waited %v before giving up — a spending cap is a human's to raise, and every extra attempt bills against it", *waited)
	}
	// One call, not one walk: ai.attemptLadder stops at the rung that named the
	// refusal rather than billing the rungs above it to the same capped account.
	if got := len(candidate.Calls()); got != 1 {
		t.Fatalf("the candidate was called %d times, want 1 — the walk stops at the refusing rung and nothing re-drives it", got)
	}
}

func TestCertifyTaskGivesUpAfterRunAttemptsAndSaysHowMany(t *testing.T) {
	waited := recordSleeps(t)
	var steps []ai.FakeStep
	for range runAttempts {
		steps = append(steps, failedWalk(t, errDroppedConnection)...)
	}
	candidate := ai.NewFakeClient().ScriptSteps(steps...)
	judge := ai.NewFakeClient().Script(scoreJSON(90))

	_, err := certifyTask(wsContext(t), ai.TaskSummarize, []Scenario{testScenario("basic", wideBands)}, testCensus(t),
		ai.ProviderConfig{Provider: ai.ProviderFake, Model: "candidate"}, ai.ProviderConfig{Provider: ai.ProviderFake, Model: "judge"},
		ai.ProfileEUHosted, 3, quietLogger(), &certifyHooks{
			candidateOpts: []ai.LocalOption{ai.WithFakeClient(candidate)},
			judgeOpts:     []ai.LocalOption{ai.WithFakeClient(judge)},
		})
	if err == nil {
		t.Fatal("a fault that never clears must still end the task rather than retry forever")
	}
	if want := fmt.Sprintf("all %d attempts", runAttempts); !strings.Contains(err.Error(), want) {
		t.Fatalf("error %q does not say how many attempts were made (want %q)", err, want)
	}
	if got, want := len(candidate.Calls()), ladderRungs(t)*runAttempts; got != want {
		t.Fatalf("the candidate was called %d times, want %d — one whole ladder walk per attempt", got, want)
	}
	if len(*waited) != runAttempts-1 {
		t.Fatalf("waited %v, want %d rising waits", *waited, runAttempts-1)
	}
	for i := 1; i < len(*waited); i++ {
		if (*waited)[i] <= (*waited)[i-1] {
			t.Fatalf("waits %v do not rise — a fault that clears on its own timescale is not helped by a flat retry", *waited)
		}
	}
}

func TestWorthRedrivingOnlyAnExhaustedLadderThatCouldClear(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want bool
	}{
		{"a dropped connection under an exhausted ladder", fmt.Errorf("%w: %w", ai.ErrAllTiersFailed, errDroppedConnection), true},
		{"a throttle under an exhausted ladder", fmt.Errorf("%w: %w", ai.ErrAllTiersFailed, ai.ErrProviderThrottled), true},
		// The router stops that walk at the refusing rung, so the refusal comes
		// back WITHOUT the sentinel — which is what makes it unretryable, rather
		// than a second exclusion here.
		{"an exhausted account, as the router reports it", fmt.Errorf("provider said no: %w", ai.ErrProviderQuota), false},
		{"a validator failure the ladder never saw", errors.New("preparing the case: bad fixture"), false},
		{"a mixed-model refusal", errors.New("refusing to certify one run answered by two models"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := worthRedriving(tc.err); got != tc.want {
				t.Fatalf("worthRedriving(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}
