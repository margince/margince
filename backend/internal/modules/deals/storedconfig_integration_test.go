// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package deals

import (
	"testing"

	"github.com/margince/margince/backend/internal/platform/config/configtest"
)

// The kill switch the environment sets stops an automatic apply over a saved
// row that allows it, and the settings surface reads it. Not parallel: the
// binding is the process's own.
func TestABootSetKillSwitchStopsAnAutoApply(t *testing.T) {
	e := setupConfigEnv(t)
	dealID, fromID, toID := twoStagePipeline(t, e)
	ref := transitionOf(t, e, fromID, toID)
	autopilotOn(t, e, true)
	ruleOn(t, e, ref)
	aGoodRecord(t, e, dealID, ref)
	if got := verdictFor(t, e, ref); got.Mode != ModeAuto {
		t.Fatalf("the fixture does not reach auto on its own (%s: %s)", got.Mode, got.Why)
	}
	configtest.Bind(t, configtest.StoredOver(t, e.pool, ConfigSettings(), "MARGINCE_DEALS_STAGE_AUTOPILOT_ENABLED=false"))

	if got := verdictFor(t, e, ref); got.Mode != ModePropose {
		t.Fatalf("a transition applied automatically with the kill switch off at boot: %s", got.Mode)
	}
	if on, err := e.store.StageAutomationEnabled(e.as()); err != nil || on {
		t.Fatalf("the settings surface reads automation %v (%v), want the off the environment sets", on, err)
	}
}

// Legal review switched off at boot releases the gate over a saved row that
// keeps it on.
func TestABootSetLegalReviewOffReleasesTheGate(t *testing.T) {
	e := setupReviewEnv(t)
	dealID, _, toID := gatedDeal(t, e)
	configtest.Bind(t, configtest.StoredOver(t, e.pool, ConfigSettings(), "MARGINCE_DEALS_LEGAL_REVIEW_ENABLED=false"))

	if err := e.advance(dealID, toID); err != nil {
		t.Fatalf("the gate held a deal with legal review off at boot: %v", err)
	}
}
