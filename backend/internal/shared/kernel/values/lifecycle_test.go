// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package values

import (
	"slices"
	"testing"
)

func TestAnAccountWeSellToOrDeliverForIsEngaged(t *testing.T) {
	want := []string{"prospect", "opportunity", "onboarding", "live", "customer"}
	if got := EngagedLifecycles(); !slices.Equal(got, want) {
		t.Fatalf("engaged lifecycles = %v, want %v", got, want)
	}
	for _, stage := range []string{"unknown", "target", "former_customer", "disqualified", "Live"} {
		if EngagedLifecycle(stage) {
			t.Errorf("%q counts as engaged", stage)
		}
	}
}
