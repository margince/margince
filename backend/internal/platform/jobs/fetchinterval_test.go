// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package jobs

import (
	"testing"
	"time"
)

// The worker keeps River's fetch timing; only a test client polls faster.
func TestOnlyATestClientPollsFasterThanRiver(t *testing.T) {
	production := riverConfig(Config{}, nil)
	if production.FetchCooldown != 0 || production.FetchPollInterval != 0 {
		t.Errorf("the worker's client fetches with cooldown %s and poll %s; it must keep River's defaults",
			production.FetchCooldown, production.FetchPollInterval)
	}
	test := riverConfig(Config{TestOnly: true}, nil)
	if test.FetchCooldown != 50*time.Millisecond || test.FetchPollInterval != 50*time.Millisecond {
		t.Errorf("a test client fetches with cooldown %s and poll %s, want 50ms each",
			test.FetchCooldown, test.FetchPollInterval)
	}
}
