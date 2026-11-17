// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package installtrial

import (
	"testing"

	"github.com/margince/margince/backend/internal/platform/licensecheck"
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
