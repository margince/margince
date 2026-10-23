// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"os"
	"regexp"
	"strconv"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/programsettings"
)

// shutdownTeardown is the room the configuration reference asks for beyond the
// drain window: the cancel wait, closing the bus and the pool, and slack.
const shutdownTeardown = 10 * time.Second

var devStopGraceLine = regexp.MustCompile(`(?m)^STOP_GRACE_SECONDS=(\d+)\s*$`)

// The dev stack's stop sends SIGKILL after STOP_GRACE_SECONDS. A shorter wait
// kills a running ingest before River can record it as stopped. Its row stays
// running, and every re-enqueue of the same work dedupes against it.
func TestTheDevStackWaitsOutTheWorkersShutdownBeforeItKills(t *testing.T) {
	t.Parallel()
	script, err := os.ReadFile("../../../scripts/dev.sh")
	if err != nil {
		t.Fatalf("reading scripts/dev.sh: %v", err)
	}
	m := devStopGraceLine.FindSubmatch(script)
	if m == nil {
		t.Fatal("scripts/dev.sh no longer sets STOP_GRACE_SECONDS on a line of its own; the stop would wait a number nothing here reads")
	}
	seconds, err := strconv.Atoi(string(m[1]))
	if err != nil {
		t.Fatalf("STOP_GRACE_SECONDS = %q: %v", m[1], err)
	}
	grace := time.Duration(seconds) * time.Second
	if need := programsettings.DefaultJobDrainWindow + jobCancelWindow; grace < need {
		t.Errorf("dev.sh waits %s before SIGKILL, shorter than the drain window and the cancel wait together (%s)", grace, need)
	}
	if need := programsettings.DefaultJobDrainWindow + shutdownTeardown; grace < need {
		t.Errorf("dev.sh waits %s before SIGKILL, shorter than the drain window plus %s of teardown (%s)", grace, shutdownTeardown, need)
	}
}
