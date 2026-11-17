// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package installtrial

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/licensecheck"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/runtimeenv"
)

// A refused license and one the module still accepts past its expiry both
// lock: there is no grace period. A valid license and no license do not; the
// trial decides the second.
func TestALicenseLapsesWhenRefusedOrPastItsExpiry(t *testing.T) {
	cases := map[string]struct {
		posture licensecheck.Posture
		lapsed  bool
	}{
		"refused":           {licensecheck.Posture{State: licensecheck.StateRejected}, true},
		"past its expiry":   {licensecheck.Posture{State: licensecheck.StateValid, License: licensecheck.License{InGrace: true}}, true},
		"valid":             {licensecheck.Posture{State: licensecheck.StateValid}, false},
		"no license at all": {licensecheck.Posture{State: licensecheck.StateAbsent}, false},
	}
	for name, c := range cases {
		if got := Lapsed(c.posture); got != c.lapsed {
			t.Errorf("%s: Lapsed = %v, want %v", name, got, c.lapsed)
		}
	}
}

// Lock decides on one posture read. A posture that turns from no license to a
// refused one between two reads must not let an ended trial through unlocked.
func TestLockDecidesOnOnePostureRead(t *testing.T) {
	reads := 0
	posture := func() licensecheck.Posture {
		reads++
		if reads == 1 {
			return licensecheck.Posture{State: licensecheck.StateAbsent}
		}
		return licensecheck.Posture{State: licensecheck.StateRejected}
	}
	started := clocktest.Now(t)
	ended := started.Add(licensecheck.TrialLength)
	trial := New(nil, runtimeenv.Production, posture, func() time.Time { return ended })
	trial.started.Store(&started)

	if err := trial.Lock(context.Background()); !errors.Is(err, apperrors.ErrTrialEnded) {
		t.Errorf("Lock = %v, want %v", err, apperrors.ErrTrialEnded)
	}
	if reads != 1 {
		t.Errorf("Lock read the posture %d times, want 1", reads)
	}
}
