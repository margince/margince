// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"errors"
	"testing"
)

// Each surface refuses a kind outside its contract enum and names only the kinds
// it takes. A hold is never told to send a container, which it refuses.
func TestABadKindIsToldOnlyTheKindsItsSurfaceTakes(t *testing.T) {
	t.Parallel()
	cases := []struct {
		surface  string
		admitted []string
		kind     string
		want     string
	}{
		{"hold", holdKinds, "company", "kind is address or domain"},
		{"hold", holdKinds, ExclusionKindContainer, "kind is address or domain"},
		{"owner identity", identityKinds, ExclusionKindContainer, "kind is address or domain"},
		{"exclusion", exclusionKinds, "company", "kind is address, domain or container"},
	}
	for _, tc := range cases {
		_, err := validRuleValue(tc.admitted, tc.kind, "gmail:Private")
		var invalid *InvalidExclusionError
		if !errors.As(err, &invalid) || invalid.Field != "kind" || invalid.Reason != tc.want {
			t.Errorf("%s with kind %q gave %v, want a refusal on kind reading %q", tc.surface, tc.kind, err, tc.want)
		}
	}
}

func TestAnExclusionStillTakesAContainer(t *testing.T) {
	t.Parallel()
	got, err := ValidExclusionValue(ExclusionKindContainer, "gmail:Private")
	if err != nil || got != "gmail:Private" {
		t.Fatalf("ValidExclusionValue(container, gmail:Private) = %q, %v; want it kept", got, err)
	}
}
