// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agentvolume

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// scriptedSource answers whatever the test last set, and counts the reads.
type scriptedSource struct {
	policy Policy
	err    error
	reads  int
}

func (s *scriptedSource) Policy(context.Context) (Policy, error) {
	s.reads++
	return s.policy, s.err
}

func sourcedMeter(src PolicySource, at *time.Time) *Meter {
	return NewWithClock(nil, Limits{}, DefaultWindow, func() time.Time { return *at }).WithPolicySource(src)
}

func TestTheMeterReadsItsSourceAtMostOncePerRefresh(t *testing.T) {
	at := time.Date(2026, 10, 14, 10, 0, 0, 0, time.UTC)
	src := &scriptedSource{policy: Policy{Limits: Limits{Writes: 10}, Window: WindowPlan{Length: DefaultWindow}}}
	meter := sourcedMeter(src, &at)

	meter.window(inWorkspace(t))
	at = at.Add(PolicyRefresh - time.Second)
	meter.window(inWorkspace(t))
	if src.reads != 1 {
		t.Fatalf("read the source %d times inside one refresh, want 1", src.reads)
	}

	src.policy.Limits.Writes = 20
	at = at.Add(time.Second)
	if got := meter.window(inWorkspace(t)).limits.Writes; got != 20 {
		t.Errorf("after the refresh the write limit is %d, want the 20 the source now answers", got)
	}
}

func TestAFailedReadKeepsTheLastGoodPolicy(t *testing.T) {
	at := time.Date(2026, 10, 14, 10, 0, 0, 0, time.UTC)
	src := &scriptedSource{policy: Policy{Limits: Limits{Writes: 10}, Window: WindowPlan{Length: DefaultWindow}}}
	meter := sourcedMeter(src, &at)
	meter.window(inWorkspace(t))

	src.err = errors.New("settings unreachable")
	at = at.Add(PolicyRefresh)

	if got := meter.window(inWorkspace(t)).limits.Writes; got != 10 {
		t.Errorf("a failed read left a write limit of %d, want the last good 10", got)
	}
}

func TestWithNoGoodPolicyTheCompiledDefaultsBind(t *testing.T) {
	at := time.Date(2026, 10, 14, 10, 0, 0, 0, time.UTC)
	meter := sourcedMeter(&scriptedSource{err: errors.New("settings unreachable")}, &at)

	w := meter.window(inWorkspace(t))

	if w.limits != (Limits{}).withDefaults() || w.length != DefaultWindow {
		t.Errorf("with no policy ever read the meter judges by %+v over %v, want the defaults", w.limits, w.length)
	}
}

func TestASourceAnsweringZeroNeverUnboundsACounter(t *testing.T) {
	at := time.Date(2026, 10, 14, 10, 0, 0, 0, time.UTC)
	meter := sourcedMeter(&scriptedSource{policy: Policy{}}, &at)

	w := meter.window(inWorkspace(t))

	if w.limits != (Limits{}).withDefaults() || w.length != DefaultWindow {
		t.Errorf("an all-zero policy became %+v over %v, want the defaults", w.limits, w.length)
	}
}

func TestAReadingQuotesTheLiveAllowanceAndLength(t *testing.T) {
	at := time.Date(2026, 10, 14, 10, 0, 0, 0, time.UTC)
	src := &scriptedSource{policy: Policy{Limits: Limits{Writes: 10}, Window: WindowPlan{Length: 6 * time.Hour}}}
	meter := sourcedMeter(src, &at)

	reading := meter.Read(principal.WithWorkspaceID(humanCtx(t), ids.NewV7()), Writes)

	if reading.Allowance != 10 || reading.Window != 6*time.Hour {
		t.Errorf("the reading quotes %d over %v, want 10 over 6h", reading.Allowance, reading.Window)
	}
	if want := bucketAt(at, 6*time.Hour); reading.Bucket != want {
		t.Errorf("the reading names bucket %d, want %d under the 6h length", reading.Bucket, want)
	}
}

// Every meter rebound from a sourced one shares its cache, so the gate and the
// tool registry never judge one call by two policies.
func TestARebindSharesTheSourceAndItsCache(t *testing.T) {
	at := time.Date(2026, 10, 14, 10, 0, 0, 0, time.UTC)
	src := &scriptedSource{policy: Policy{Limits: Limits{Writes: 10}, Window: WindowPlan{Length: DefaultWindow}}}
	live := sourcedMeter(src, &at)
	held := Unmetered()

	held.RebindFrom(live)
	live.window(inWorkspace(t))
	held.window(inWorkspace(t))

	if src.reads != 1 {
		t.Errorf("two rebound meters read the source %d times, want 1", src.reads)
	}
}

// A blip costs seconds, not the minute a good read is trusted for.
func TestAFailedReadIsRetriedSoonerThanARefresh(t *testing.T) {
	at := time.Date(2026, 10, 14, 10, 0, 0, 0, time.UTC)
	src := &scriptedSource{err: errors.New("settings unreachable")}
	meter := sourcedMeter(src, &at)
	meter.window(inWorkspace(t))

	src.err, src.policy = nil, Policy{Limits: Limits{Writes: 10}, Window: WindowPlan{Length: DefaultWindow}}
	at = at.Add(policyRetry)

	if got := meter.window(inWorkspace(t)).limits.Writes; got != 10 {
		t.Errorf("after the retry interval the write limit is %d, want the 10 the source now answers", got)
	}
}

// One request giving up must not cost every other call the admin's policy.
func TestACancelledCallStillReadsThePolicy(t *testing.T) {
	at := time.Date(2026, 10, 14, 10, 0, 0, 0, time.UTC)
	src := &cancelAwareSource{policy: Policy{Limits: Limits{Writes: 10}, Window: WindowPlan{Length: DefaultWindow}}}
	meter := NewWithClock(nil, Limits{}, DefaultWindow, func() time.Time { return at }).WithPolicySource(src)
	ctx, cancel := context.WithCancel(inWorkspace(t))
	cancel()

	if got := meter.window(ctx).limits.Writes; got != 10 {
		t.Errorf("a cancelled caller left the write limit at %d, want 10", got)
	}
}

type cancelAwareSource struct{ policy Policy }

func (s *cancelAwareSource) Policy(ctx context.Context) (Policy, error) {
	if err := ctx.Err(); err != nil {
		return Policy{}, err
	}
	return s.policy, nil
}

// The process that saved the allowances binds them on its next call.
func TestForgettingThePolicyReadsTheSourceAgain(t *testing.T) {
	at := time.Date(2026, 10, 14, 10, 0, 0, 0, time.UTC)
	src := &scriptedSource{policy: Policy{Limits: Limits{Writes: 200}, Window: WindowPlan{Length: DefaultWindow}}}
	meter := sourcedMeter(src, &at)
	meter.window(inWorkspace(t))

	src.policy.Limits.Writes = 10
	meter.ForgetPolicy()

	if got := meter.window(inWorkspace(t)).limits.Writes; got != 10 {
		t.Errorf("after ForgetPolicy the write limit is %d, want the 10 just saved", got)
	}
}

// A save that lands while a refresh is reading must not be hidden behind the
// older answer that refresh brings back.
func TestARefreshInFlightDoesNotHideASave(t *testing.T) {
	at := time.Date(2026, 10, 14, 10, 0, 0, 0, time.UTC)
	src := &forgettingSource{policy: Policy{Limits: Limits{Writes: 200}, Window: WindowPlan{Length: DefaultWindow}}}
	meter := sourcedMeter(src, &at)
	meter.window(inWorkspace(t))
	at = at.Add(PolicyRefresh)

	// The refresh reads 200 while an admin saves 10 and the saving process forgets.
	src.during = func() { src.policy.Limits.Writes = 10; meter.ForgetPolicy() }
	meter.window(inWorkspace(t))
	src.during = nil

	if got := meter.window(inWorkspace(t)).limits.Writes; got != 10 {
		t.Errorf("after a save during a refresh the write limit is %d, want the 10 saved", got)
	}
}

// forgettingSource answers its policy as it was when asked, then runs during.
type forgettingSource struct {
	policy Policy
	during func()
}

func (s *forgettingSource) Policy(context.Context) (Policy, error) {
	answer := s.policy
	if s.during != nil {
		s.during()
	}
	return answer, nil
}

// inWorkspace is a call bound to a workspace, which a settings read needs.
func inWorkspace(t *testing.T) context.Context {
	t.Helper()
	return principal.WithWorkspaceID(t.Context(), ids.NewV7())
}

// A call bound to no workspace cannot read the settings. It must not count as
// a failed read that pushes every other call onto the defaults.
func TestACallWithNoWorkspaceLeavesThePolicyAlone(t *testing.T) {
	at := time.Date(2026, 10, 14, 10, 0, 0, 0, time.UTC)
	src := &scriptedSource{policy: Policy{Limits: Limits{Writes: 10}, Window: WindowPlan{Length: DefaultWindow}}}
	meter := sourcedMeter(src, &at)

	meter.window(t.Context())

	if src.reads != 0 {
		t.Fatalf("a call with no workspace read the source %d times", src.reads)
	}
	if got := meter.window(inWorkspace(t)).limits.Writes; got != 10 {
		t.Errorf("the next bound call judges by %d writes, want the 10 the source answers", got)
	}
}
