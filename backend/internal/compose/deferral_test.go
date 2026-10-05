// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

func TestDeferralBackoffWaitsForTheProvidersProbeNotTheFixedSpacing(t *testing.T) {
	const fixed = 30 * time.Minute
	down := &ai.ProviderDownError{Provider: "acme", Health: model.HealthDown, RetryAfter: time.Now().Add(time.Hour)}

	if got := deferralBackoff(fmt.Errorf("wrapped: %w", down), fixed); got < 59*time.Minute || got > time.Hour {
		t.Errorf("provider outage backoff = %v, want the hour until its probe", got)
	}
	if got := deferralBackoff(ai.ErrBudgetDeferred, fixed); got != fixed {
		t.Errorf("budget stop backoff = %v, want the lane's fixed %v", got, fixed)
	}
	past := &ai.ProviderDownError{Provider: "acme", Health: model.HealthDown, RetryAfter: time.Now().Add(-time.Minute)}
	if got := deferralBackoff(past, fixed); got != 0 {
		t.Errorf("a probe already due backs off %v, want none", got)
	}
}

func TestSweepPausedEndsThePassForAnOutageButNotForAnOrdinaryFailure(t *testing.T) {
	down := &ai.ProviderDownError{Provider: "acme", Health: model.HealthOutOfCredit, RetryAfter: time.Now()}
	for name, err := range map[string]error{
		"provider outage": down, "budget stop": ai.ErrBudgetDeferred, "no provider": ai.ErrUnconfiguredModel,
	} {
		if !sweepPaused(err) {
			t.Errorf("%s did not pause the sweep", name)
		}
	}
	if sweepPaused(errors.New("provider said: internal error")) {
		t.Error("an ordinary failure paused the sweep instead of failing the pass")
	}
	if declinedByTheModels(down) {
		t.Error("an outage was recorded as the models declining a message")
	}
}
