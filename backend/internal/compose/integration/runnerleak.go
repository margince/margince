// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package integration

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/jobs"
)

// jobRunnerDrainTimeout is the drain a stopping runner is given. Generous
// because a saturated runner is the case that needs it, and waiting costs
// nothing next to giving up early.
const jobRunnerDrainTimeout = 30 * time.Second

// liveRunners counts each test's undrained runners. A runner still live when
// its test ends loses its database mid-job, so the test that leaked it fails.
var liveRunners = struct {
	sync.Mutex
	byTest map[string]int
}{byTest: map[string]int{}}

// StartedJobRunner records a running runner and answers the function that stops
// it. One function because five tests spelled this drain themselves.
func StartedJobRunner(t *testing.T, runner *jobs.Runner) func() {
	t.Helper()
	drained := runnerStarted(t)
	return func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), jobRunnerDrainTimeout)
		defer cancel()
		if err := runner.Stop(stopCtx); err != nil {
			t.Errorf("Stop: %v — its workers are still running", err)
			return
		}
		drained()
	}
}

// runnerStarted counts one live runner against this test. The returned function
// says it drained. The check runs after every later cleanup of the test.
func runnerStarted(t *testing.T) func() {
	t.Helper()
	liveRunners.Lock()
	first := liveRunners.byTest[t.Name()] == 0
	liveRunners.byTest[t.Name()]++
	liveRunners.Unlock()
	if first {
		t.Cleanup(func() {
			liveRunners.Lock()
			defer liveRunners.Unlock()
			if count := liveRunners.byTest[t.Name()]; count > 0 {
				t.Errorf("%d job runner(s) outlived the test that started them: drain each in the test's own cleanup", count)
			}
			delete(liveRunners.byTest, t.Name())
		})
	}
	return func() {
		liveRunners.Lock()
		defer liveRunners.Unlock()
		liveRunners.byTest[t.Name()]--
	}
}
