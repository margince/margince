// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"testing"

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
