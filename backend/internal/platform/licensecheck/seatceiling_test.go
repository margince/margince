// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package licensecheck

import (
	"context"
	"log/slog"
	"testing"

	"github.com/margince/margince/backend/internal/shared/runtimeenv"
)

// refusedAfter boots an unlicensed watcher, stands in the posture it held
// before, and re-checks against a token the module refuses.
func refusedAfter(t *testing.T, held Posture) Posture {
	t.Helper()
	source := &tokens{}
	w, err := NewWatcher(context.Background(), source.source, fixedClock(checkedAt), slog.New(slog.DiscardHandler), runtimeenv.Production)
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}
	w.posture.Store(&held)
	source.token = "no-longer-honored"
	w.Recheck(context.Background())
	got := w.Posture()
	if got.State != StateRejected {
		t.Fatalf("posture = %q, want %q, so this test reads no refusal", got.State, StateRejected)
	}
	return got
}

// A license refused while the process runs keeps the last valid ceiling, so no
// seat is admitted beyond what that license granted.
func TestARefusedLicenseKeepsTheLastValidSeatCeiling(t *testing.T) {
	t.Parallel()
	got := refusedAfter(t, Posture{State: StateValid, Grants: Grants{SeatsAttribute: float64(25)}})
	if seats, capped := got.SeatCeiling(); !capped || seats != 25 {
		t.Errorf("ceiling after a refusal = %d (capped %v), want the last valid 25", seats, capped)
	}
	if _, granted := got.Seats(); granted {
		t.Error("a refused license reports a grant of its own")
	}
}

// The held ceiling survives a second refusal, and a last valid license that
// capped nothing leaves nothing capped.
func TestTheHeldCeilingIsTheLastValidOneWhateverItWas(t *testing.T) {
	t.Parallel()
	twice := refusedAfter(t, refusedAfter(t, Posture{State: StateValid, Grants: Grants{SeatsAttribute: float64(3)}}))
	if seats, capped := twice.SeatCeiling(); !capped || seats != 3 {
		t.Errorf("ceiling after two refusals = %d (capped %v), want 3", seats, capped)
	}
	uncapped := refusedAfter(t, Posture{State: StateValid, Grants: Grants{"feature": true}})
	if _, capped := uncapped.SeatCeiling(); capped {
		t.Error("a refusal after a license that capped nothing invented a ceiling")
	}
}

// A process that never resolved a valid license has no ceiling to keep.
func TestARefusalWithNoValidLicenseBeforeItCapsNothing(t *testing.T) {
	t.Parallel()
	got := refusedAfter(t, Posture{State: StateAbsent})
	if _, capped := got.SeatCeiling(); capped {
		t.Error("a refusal with no valid license before it invented a ceiling")
	}
}
