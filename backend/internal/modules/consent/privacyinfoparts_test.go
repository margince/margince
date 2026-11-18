// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// TestAReaderWithNothingOnRecordIsToldNoBasisIsRecorded: under a pack with no
// legitimate-interest basis, the page never claims an agreement that is not on
// record. Consent or an agreement on record keeps its basis.
func TestAReaderWithNothingOnRecordIsToldNoBasisIsRecorded(t *testing.T) {
	for _, c := range []struct {
		name        string
		noInterests bool
		onRecord    commsauthz.Basis
		want        string
	}{
		{"nothing on record under such a pack", true, "", noBasisRecorded},
		{"an agreement on record", true, commsauthz.BasisContract, "contract"},
		{"consent on record", true, commsauthz.BasisConsent, "consent"},
		{"a pack with legitimate interests", false, "", "legitimate_interests"},
	} {
		if got := defaultUseBasis(c.noInterests, c.onRecord); got != c.want {
			t.Errorf("%s: basis %q, want %q", c.name, got, c.want)
		}
	}
}
