// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// apiFlagOnly holds the boot flags this role keeps off the environment, with the
// reason. Anything else a deployment must be able to set from its container's
// environment, where a flag alone would mean editing the command line.
var apiFlagOnly = gatekit.Waive(map[string]string{
	"ai-fake": "the offline fake model is for dev and test; a variable left set in a deployment's environment would serve fake answers in production",
})

func TestEveryAPIBootFlagHasAnEnvironmentName(t *testing.T) {
	fs, env, _, err := apiFlagSet()
	if err != nil {
		t.Fatal(err)
	}
	keptOff := func(name string) bool { return apiFlagOnly.Waived(t, name) }
	for _, shortfall := range env.Shortfalls(fs, keptOff) {
		t.Error(shortfall)
	}
	apiFlagOnly.AssertAllMatched(t)
}

// A value the environment holds but its flag cannot read is reported with the
// other faults of the same boot, not instead of them.
func TestAnUnreadableEnvironmentValueJoinsTheBootFaults(t *testing.T) {
	t.Setenv("MARGINCE_DSN", "")
	t.Setenv("MARGINCE_INLINE_RELAY", "maybe")
	_, err := parseAPIFlags(nil)
	if err == nil {
		t.Fatal("parsing answered no error, want both faults")
	}
	for _, want := range []string{"--dsn", "MARGINCE_INLINE_RELAY"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the error does not mention %s:\n%s", want, err)
		}
	}
}
