// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"
)

// answers replays one installation's recorded release, tick by tick, so the
// three sequences that matter are three fixtures rather than three deployments.
func answers(seq ...answer) (func(context.Context) (string, error), <-chan struct{}) {
	// Announced rather than timed: a test that slept would be asserting about a
	// clock, and what it means to say is "after the watcher had read N times it
	// still had not fired". Buffered generously so a reader never blocks on a
	// test that has stopped counting.
	reads := make(chan struct{}, 64)
	i := 0
	return func(context.Context) (string, error) {
		at := seq[len(seq)-1]
		if i < len(seq) {
			at = seq[i]
			i++
		}
		// Past the fixture the last entry repeats, which is what a settled
		// installation looks like.
		select {
		case reads <- struct{}{}:
		default:
		}
		return at.recorded, at.err
	}, reads
}

// afterReads waits until the watcher has read the record n times, so what
// follows is a statement about those reads rather than about elapsed time.
func afterReads(t *testing.T, reads <-chan struct{}, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		select {
		case <-reads:
		case <-time.After(2 * time.Second):
			t.Fatalf("the watcher stopped reading after %d of %d reads", i, n)
		}
	}
}

type answer struct {
	recorded string
	err      error
}

// quiet keeps a watcher's own logging out of the test output; what is under
// test is what it does, not what it says.
func quiet() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// waitFor reads the watcher's answer, or reports that it never came.
func waitFor(t *testing.T, skew <-chan error, within time.Duration) (error, bool) {
	t.Helper()
	select {
	case err, open := <-skew:
		return err, open
	case <-time.After(within):
		t.Fatal("the watcher neither refused nor closed")
		return nil, false
	}
}

// A record that has SETTLED on another release stops the role.
func TestASettledDifferenceStopsTheRole(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	read, _ := answers(answer{recorded: "1970.43"})
	skew := watchRelease(ctx, quiet(), "1970.42", time.Millisecond, read)
	err, open := waitFor(t, skew, 2*time.Second)
	if !open || err == nil {
		t.Fatal("a role running against another release's schema was not stopped")
	}
	// The operator reads the same sentence the boot guard gives, because it is
	// the same fact arriving later.
	for _, want := range []string{"1970.42", "1970.43", "Deploy every role"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal does not mention %q: %v", want, err)
		}
	}
}

// A FLAP does not. During a rolling api deploy two releases are alive and each
// records its own, so the record alternates for the length of the rollout — and
// a watcher that fired on one differing read would restart every worker in the
// fleet, repeatedly, for the duration.
func TestAFlappingRecordDoesNotStopTheRole(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	read, reads := answers(
		answer{recorded: "1970.43"},
		answer{recorded: "1970.42"},
		answer{recorded: "1970.43"},
		answer{recorded: "1970.42"},
		answer{recorded: "1970.43"},
		answer{recorded: "1970.42"},
	)
	skew := watchRelease(ctx, quiet(), "1970.42", time.Millisecond, read)
	// Twice the fixture, which is many more ticks than a confirmation run needs.
	afterReads(t, reads, 12)
	select {
	case err := <-skew:
		t.Fatalf("a rolling deploy stopped the role: %v", err)
	default:
	}
}

// An UNREADABLE record does not, and this is the rule that matters most: a
// momentary database error is not a release change, and a guard that exited on
// one would be an outage lever pointed at the deployment it protects.
func TestAnUnreadableRecordDoesNotStopTheRole(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	read, reads := answers(answer{err: errors.New("connection refused")})
	skew := watchRelease(ctx, quiet(), "1970.42", time.Millisecond, read)
	afterReads(t, reads, 12)
	select {
	case err := <-skew:
		t.Fatalf("a database error stopped the role: %v", err)
	default:
	}
}

// A read failure neither confirms a difference nor clears one: it says nothing
// about the record. So a difference interrupted by an outage still stops the
// role once it has been seen enough times.
func TestAReadFailureNeitherConfirmsNorClearsADifference(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// The sequence ENDS on a read failure, and answers repeats its last entry
	// forever — so a watcher that cleared the count on a failure can never
	// reach three and this test discriminates. Ending on the difference
	// instead, it would reach three from the repeats alone and pass either way.
	read, _ := answers(
		answer{recorded: "1970.43"},
		answer{err: errors.New("connection refused")},
		answer{recorded: "1970.43"},
		answer{err: errors.New("connection refused")},
		answer{recorded: "1970.43"},
		answer{err: errors.New("connection refused")},
	)
	skew := watchRelease(ctx, quiet(), "1970.42", time.Millisecond, read)
	if err, open := waitFor(t, skew, 2*time.Second); !open || err == nil {
		t.Fatal("a settled difference interrupted by two read failures did not stop the role")
	}
}

// An installation that has recorded nothing is not a difference. A worker can
// legitimately reach a pre-bootstrap installation, and the boot guard already
// says so once.
func TestAnUnrecordedInstallationIsNotADifference(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	read, reads := answers(answer{recorded: ""})
	skew := watchRelease(ctx, quiet(), "1970.42", time.Millisecond, read)
	afterReads(t, reads, 12)
	select {
	case err := <-skew:
		t.Fatalf("an unrecorded installation stopped the role: %v", err)
	default:
	}
}

// A binary with no comparable release watches nothing, and says so by closing
// immediately rather than ticking forever against a comparison it cannot make.
func TestABinaryWithNoReleaseWatchesNothing(t *testing.T) {
	skew := watchRelease(context.Background(), quiet(), "", time.Millisecond,
		func(context.Context) (string, error) {
			t.Error("a binary with no release read the record")
			return "1970.43", nil
		})
	if err, open := waitFor(t, skew, time.Second); open || err != nil {
		t.Errorf("the channel answered %v (open=%t), want closed with nothing", err, open)
	}
}

// Shutdown closes the channel with nothing, which is how the caller tells an
// ordinary stop from a refusal.
func TestShutdownClosesWithNoRefusal(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	read, _ := answers(answer{recorded: "1970.42"})
	skew := watchRelease(ctx, quiet(), "1970.42", time.Hour, read)
	cancel()
	if err, open := waitFor(t, skew, time.Second); open || err != nil {
		t.Errorf("shutdown answered %v (open=%t), want closed with nothing", err, open)
	}
}
