// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package promiseband

import "testing"

func TestAReadingIsFirmTentativeOrDroppedByItsConfidence(t *testing.T) {
	for confidence, want := range map[float64]Band{
		1:    Firm,
		0.9:  Firm,
		0.7:  Firm,
		0.69: Tentative,
		0.3:  Tentative,
		0.29: Dropped,
		0:    Dropped,
	} {
		if got := Of(confidence); got != want {
			t.Errorf("a reading at %v is %s, want %s", confidence, got, want)
		}
	}
}
