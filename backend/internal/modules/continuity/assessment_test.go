// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package continuity

import (
	"testing"
	"time"
)

func TestRestoreVerificationRequiresTheDrillsOwnOutcomeAndWindows(t *testing.T) {
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	finished := at.Add(RecoveryTarget)
	drill := Drill{StartedAt: at, FinishedAt: &finished, RestoredTo: at.Add(-DataLossTarget), Outcome: OutcomePassed}
	if AssessDrill(nil).Code != "recovery_missing" {
		t.Fatal("missing evidence was verified")
	}
	if AssessDrill(&drill).Code != "recovery_verified" {
		t.Fatal("target boundary should pass")
	}
	finished = finished.Add(time.Second)
	if AssessDrill(&drill).Code != "recovery_unverified" {
		t.Fatal("over-target recovery was verified")
	}
	finished = at.Add(RecoveryTarget)
	drill.Outcome = OutcomeFailed
	if AssessDrill(&drill).Code != "recovery_unverified" {
		t.Fatal("failed drill was verified")
	}
}
