// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

// Shutdown's one promise: nothing this process started is still running when it
// closes the bus and the pool that thing writes through.
//
// Both halves used to break it in the same way and for opposite reasons. The
// job drain RETURNED on its deadline with job goroutines still going, because
// that is what River's Stop does when its context expires. The lane join had no
// deadline at all, so a handler that ignored cancellation hung the process
// instead — which on the boot-failure path means an `exit 1` that never
// happens and a supervisor that reads a dead worker as still starting.

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeJobLane records which stop it was asked for and answers each as the test
// arranges. The real overrun cannot be staged without a job that will not
// finish, which is the one thing a test cannot arrange without waiting for it.
type fakeJobLane struct {
	stopErr       error
	cancelErr     error
	stops         int
	stopAndCancel int
}

func (f *fakeJobLane) Stop(context.Context) error {
	f.stops++
	return f.stopErr
}

func (f *fakeJobLane) StopAndCancel(context.Context) error {
	f.stopAndCancel++
	return f.cancelErr
}

func recordingLogger() (*slog.Logger, *bytes.Buffer) {
	var written bytes.Buffer
	return slog.New(slog.NewTextHandler(&written, &slog.HandlerOptions{Level: slog.LevelDebug})), &written
}

func TestADrainThatFinishesIsNotEscalated(t *testing.T) {
	lane := &fakeJobLane{}
	logger, written := recordingLogger()

	stopJobRunner(t.Context(), lane, logger)

	if lane.stopAndCancel != 0 {
		t.Errorf("a drain that finished was escalated to StopAndCancel %d time(s); "+
			"cancelling a job that was about to complete costs it a retry for nothing", lane.stopAndCancel)
	}
	if written.Len() != 0 {
		t.Errorf("an ordinary shutdown logged %q; nothing went wrong", written.String())
	}
}

// The window expiring is exactly what River reports, and it is NOT the end of
// the story: Stop returns, the job goroutines do not.
func TestADrainThatOverrunsCancelsTheJobsStillRunning(t *testing.T) {
	lane := &fakeJobLane{stopErr: context.DeadlineExceeded}
	logger, written := recordingLogger()

	stopJobRunner(t.Context(), lane, logger)

	if lane.stopAndCancel != 1 {
		t.Fatalf("the drain overran and StopAndCancel was called %d time(s), want 1 — "+
			"without it the job goroutines outlive the bus and the pool this process is closing", lane.stopAndCancel)
	}
	if !strings.Contains(written.String(), "level=WARN") {
		t.Errorf("the escalation was silent: %q", written.String())
	}
}

// The floor. Both stops overran, the connections close under live job
// goroutines, and the only thing left to do is say so where it can be read as
// the cause rather than as a symptom.
func TestJobsThatSurviveBothStopsAreReportedAsAnError(t *testing.T) {
	lane := &fakeJobLane{stopErr: context.DeadlineExceeded, cancelErr: errors.New("still working")}
	logger, written := recordingLogger()

	stopJobRunner(t.Context(), lane, logger)

	if !strings.Contains(written.String(), "level=ERROR") {
		t.Errorf("this process is closing its connections under running jobs and said nothing at ERROR: %q", written.String())
	}
}

func TestJoinReportsALaneThatDoesNotStop(t *testing.T) {
	logger, written := recordingLogger()
	_, stop := context.WithCancel(context.Background())
	var background sync.WaitGroup

	// A lane that ignores cancellation. It is released at the end of the test
	// rather than left running, so the suite does not leak it.
	stuck := make(chan struct{})
	t.Cleanup(func() { close(stuck) })
	background.Go(func() { <-stuck })

	returned := make(chan struct{})
	go func() {
		joinLanesWithin(stop, &background, logger, time.Millisecond)
		close(returned)
	}()

	select {
	case <-returned:
	case <-time.After(5 * time.Second):
		t.Fatal("join() never returned with a lane that ignores cancellation — a boot failure that " +
			"cannot exit is read by a supervisor as a worker that is still starting")
	}
	if !strings.Contains(written.String(), "level=ERROR") {
		t.Errorf("join() abandoned a running lane silently: %q", written.String())
	}
}
