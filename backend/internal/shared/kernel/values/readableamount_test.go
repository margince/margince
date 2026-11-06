// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package values

import (
	"slices"
	"testing"
)

func TestAMinorAmountIsShownBesideItsCurrency(t *testing.T) {
	for _, tc := range []struct {
		member, display string
		minor           bool
	}{
		{"total_minor", "total_display", true},
		{"amount_minor", "amount_display", true},
		{"_minor", "", false},
		{"total", "", false},
		{"minor_total", "", false},
	} {
		display, minor := DisplayMemberOf(tc.member)
		if display != tc.display || minor != tc.minor {
			t.Errorf("DisplayMemberOf(%q) = %q, %v; want %q, %v", tc.member, display, minor, tc.display, tc.minor)
		}
	}
	if got := AmountCurrencyMembers(); !slices.Equal(got, []string{"currency", "base_currency"}) {
		t.Errorf("an amount names its currency by %v", got)
	}
}
