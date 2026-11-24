// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"strings"
	"testing"
)

func TestADayZoneIsABoundTextParameterAndAnUnknownOneBindsNothing(t *testing.T) {
	var bound []any
	arg := func(v any) int { bound = append(bound, v); return len(bound) }
	if got := DayZoneSQL("Asia/Ho_Chi_Minh", arg); got != "$1::text" || len(bound) != 1 {
		t.Errorf("a known zone gave %q with %d arguments, want $1::text and one", got, len(bound))
	}
	bound = nil
	for _, zone := range []string{"", "Not/AZone", "x'; DROP TABLE activity; --"} {
		got := DayZoneSQL(zone, arg)
		if !strings.Contains(got, "installation.timezone") || len(bound) != 0 {
			t.Errorf("zone %q gave %q with %d arguments, want the installation's zone and none", zone, got, len(bound))
		}
	}
}
