// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// workerFlagOnly holds the boot flags this role keeps off the environment, with
// the reason. Anything else a deployment must be able to set from its
// container's environment, where a flag alone would mean editing the command
// line.
var workerFlagOnly = gatekit.Waive(map[string]string{
	"ai-fake": "the offline fake model is for dev and test; a variable left set in a deployment's environment would run the agent scheduler on fake answers in production",
})

func TestEveryWorkerBootFlagHasAnEnvironmentName(t *testing.T) {
	fs, env, _, err := workerFlagSet()
	if err != nil {
		t.Fatal(err)
	}
	keptOff := func(name string) bool { return workerFlagOnly.Waived(t, name) }
	for _, shortfall := range env.Shortfalls(fs, keptOff) {
		t.Error(shortfall)
	}
	workerFlagOnly.AssertAllMatched(t)
}

// An interval set in the environment reaches the role, and one it cannot read
// refuses the boot naming the variable rather than running the default.
func TestAnIntervalComesFromTheEnvironmentOrRefusesTheBoot(t *testing.T) {
	base := []string{"--dsn", "postgres://localhost/x"}
	t.Setenv("MARGINCE_RUNNER_INTERVAL", "45s")
	cfg, err := parseWorkerFlags(base)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.runnerInterval != 45*time.Second {
		t.Errorf("MARGINCE_RUNNER_INTERVAL=45s produced %s", cfg.runnerInterval)
	}

	t.Setenv("MARGINCE_RUNNER_INTERVAL", "often")
	if _, err := parseWorkerFlags(base); err == nil || !strings.Contains(err.Error(), "MARGINCE_RUNNER_INTERVAL") {
		t.Errorf("MARGINCE_RUNNER_INTERVAL=often gave %v, want a boot error naming the variable", err)
	}
}
