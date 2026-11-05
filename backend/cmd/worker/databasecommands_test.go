// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"strings"
	"testing"
)

// Every database command's line loads the worker's settings. The worker's own
// flag parse never sees the command's name or flags.
func TestEveryDatabaseCommandLineLoadsTheWorkerSettings(t *testing.T) {
	t.Parallel()
	for name := range databaseCommands {
		line := []string{name, "--workspace=00000000-0000-0000-0000-000000000000"}
		if _, _, err := loadWorkerConfig(workerFlags(line), []string{workerTestDSN}); err != nil {
			t.Errorf("worker %s: %v, want its settings loaded", name, err)
		}
	}
}

// A word that names no command still reaches the flag parse and is refused.
func TestTheWorkerRefusesAnUnknownCommand(t *testing.T) {
	t.Parallel()
	_, _, err := loadWorkerConfig(workerFlags([]string{"replay-erasure"}), []string{workerTestDSN})
	if err == nil || !strings.Contains(err.Error(), "unexpected argument at position 1") {
		t.Fatalf("worker replay-erasure = %v, want the stray word refused", err)
	}
}
