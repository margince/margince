// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/programsettings"
	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// shutdownSlack is what the configuration reference asks for beyond the drain
// window and the cancel wait: closing the bus and the pool, and some margin.
const shutdownSlack = 5 * time.Second

var (
	devStopGraceLine = regexp.MustCompile(`(?m)^STOP_GRACE_SECONDS=(\d+)\s*$`)
	devKillWaitLoop  = regexp.MustCompile(`for _ in \$\(seq 1 \$\(\(STOP_GRACE_SECONDS \* 2\)\)\); do`)
)

// The dev stack's stop sends SIGKILL after STOP_GRACE_SECONDS. A shorter wait
// kills a running ingest before River can record it as stopped, and its row
// stays running.
func TestTheDevStackWaitsOutTheWorkersShutdownBeforeItKills(t *testing.T) {
	t.Parallel()
	if err := gatekit.DeclareInputs(os.Getenv, "../../../scripts"); err != nil {
		t.Fatalf("declaring the scripts tree this test reads: %v", err)
	}
	raw, err := os.ReadFile("../../../scripts/dev.sh")
	if err != nil {
		t.Fatalf("reading scripts/dev.sh: %v", err)
	}
	script := string(raw)
	m := devStopGraceLine.FindStringSubmatch(script)
	if m == nil {
		t.Fatal("scripts/dev.sh no longer sets STOP_GRACE_SECONDS on a line of its own; the stop would wait a number nothing here reads")
	}
	seconds, err := strconv.Atoi(m[1])
	if err != nil {
		t.Fatalf("STOP_GRACE_SECONDS = %q: %v", m[1], err)
	}
	if need := programsettings.DefaultJobDrainWindow + jobCancelWindow + shutdownSlack; time.Duration(seconds)*time.Second < need {
		t.Errorf("dev.sh waits %ds before SIGKILL, shorter than the drain window, the cancel wait and the teardown slack together (%s)", seconds, need)
	}

	_, afterName, found := strings.Cut(script, "\nkill_pids()")
	if !found {
		t.Fatal("scripts/dev.sh no longer defines kill_pids, so there is no wait to check")
	}
	killPids, _, _ := strings.Cut(afterName, "\n}\n")
	if !devKillWaitLoop.MatchString(killPids) {
		t.Error("kill_pids waits a number of its own, not STOP_GRACE_SECONDS, so the constant above decides nothing")
	}
}
