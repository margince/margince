// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package integration

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/jobs"
)

// jobRunnerDrainTimeout is the drain a stopping runner is given. Generous
// because a saturated runner is the case that needs it, and waiting costs
// nothing next to giving up early.
const jobRunnerDrainTimeout = 30 * time.Second

// Every test in a lane shares one database, and Setup resets it. So workers a
// test failed to drain wake up under the next test's rows.
//
// The test that then fails is never the one that leaked them. A leak is
// reported against its own test instead, at the next Setup.
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
// says it drained. A test that never calls it has leaked workers, and the next
// Setup refuses to reset the database under them.
func runnerStarted(t *testing.T) func() {
	t.Helper()
	liveRunners.Lock()
	liveRunners.byTest[t.Name()]++
	liveRunners.Unlock()
	return func() {
		liveRunners.Lock()
		defer liveRunners.Unlock()
		if liveRunners.byTest[t.Name()] <= 1 {
			delete(liveRunners.byTest, t.Name())
			return
		}
		liveRunners.byTest[t.Name()]--
	}
}

// leakedRunners names the tests holding undrained runners.
func leakedRunners() string {
	liveRunners.Lock()
	defer liveRunners.Unlock()
	if len(liveRunners.byTest) == 0 {
		return ""
	}
	out := ""
	for name, count := range liveRunners.byTest {
		out += fmt.Sprintf("\n  %s holds %d undrained job runner(s)", name, count)
	}
	return out
}
