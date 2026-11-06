// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package weekly

import "testing"

// The retired bars fold into "other", their money and their deals with them.
func TestTheRetiredBarsFoldIntoOther(t *testing.T) {
	t.Parallel()
	folded := foldRetiredBars([]Movement{
		{PeriodKind: "quarter", Bar: BarCreated, DeltaMinor: 100},
		{PeriodKind: "quarter", Bar: "excluded", DeltaMinor: -40, Drivers: []Driver{{DealLabel: "A", DeltaMinor: -40}}},
		{PeriodKind: "quarter", Bar: "reinstated", DeltaMinor: 15, Drivers: []Driver{{DealLabel: "B", DeltaMinor: 15}}},
	})
	if len(folded) != 2 || folded[1].Bar != BarOther || folded[1].DeltaMinor != -25 {
		t.Fatalf("folded = %+v, want created and one other bar of -25", folded)
	}
	if len(folded[1].Drivers) != 2 || folded[1].Drivers[0].DealLabel != "A" {
		t.Fatalf("drivers = %+v, want both, largest first", folded[1].Drivers)
	}
	crowded := foldRetiredBars([]Movement{
		{PeriodKind: "quarter", Bar: BarOther, Drivers: []Driver{{DealLabel: "A", DeltaMinor: 1}, {DealLabel: "B", DeltaMinor: 2}}},
		{PeriodKind: "quarter", Bar: "excluded", Drivers: []Driver{{DealLabel: "C", DeltaMinor: -3}, {DealLabel: "D", DeltaMinor: -9}}},
		{PeriodKind: "quarter", Bar: "reinstated", Drivers: []Driver{{DealLabel: "E", DeltaMinor: 8}}},
	})
	if drivers := crowded[0].Drivers; len(drivers) != driversPerBar || drivers[0].DealLabel != "D" || drivers[2].DealLabel != "C" {
		t.Fatalf("drivers = %+v, want the %d largest of the merged bars", drivers, driversPerBar)
	}
	merged := foldRetiredBars([]Movement{
		{PeriodKind: "quarter", Bar: BarOther, DeltaMinor: 7},
		{PeriodKind: "quarter", Bar: "not_checked", DeltaMinor: 3},
	})
	if len(merged) != 1 || merged[0].DeltaMinor != 10 {
		t.Fatalf("merged = %+v, want one other bar of 10", merged)
	}
}
