// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package vacuumhealth

import "testing"

func width(w float64) *float64 { return &w }

// 1000 rows of 40 bytes take 68 bytes each with header, padding and pointer,
// so 120 fit in a full 8 kB page and 9 pages hold them.
func TestTheLiveRowsLeaveTheRestOfTheFileAsWaste(t *testing.T) {
	reading := tableReading{tableBytes: 100 * 8192, liveRows: 1000, rowWidth: width(40), fillfactor: 100, blockSize: 8192}
	waste, known := reading.wasteBytes()
	if !known || waste != 91*8192 {
		t.Fatalf("waste = %d (known %v), want %d", waste, known, 91*8192)
	}
}

func TestAFillfactorUnderAHundredIsSpaceTheTableAskedFor(t *testing.T) {
	full := tableReading{tableBytes: 100 * 8192, liveRows: 1000, rowWidth: width(40), fillfactor: 100, blockSize: 8192}
	half := full
	half.fillfactor = 50
	fullWaste, _ := full.wasteBytes()
	halfWaste, _ := half.wasteBytes()
	if halfWaste >= fullWaste {
		t.Fatalf("fillfactor 50 leaves waste %d, want under the %d of a full packing", halfWaste, fullWaste)
	}
}

func TestATableWithoutStatisticsHasNoEstimate(t *testing.T) {
	for name, reading := range map[string]tableReading{
		"never analyzed":       {tableBytes: 8192, liveRows: -1, rowWidth: width(40), fillfactor: 100, blockSize: 8192},
		"rows without a width": {tableBytes: 8192, liveRows: 10, fillfactor: 100, blockSize: 8192},
	} {
		if _, known := reading.wasteBytes(); known {
			t.Errorf("%s: an estimate was made from no statistics", name)
		}
	}
}

// An analyzed table with every row deleted has no column statistics at all,
// and the whole file is waste.
func TestAnEmptiedTableIsAllWaste(t *testing.T) {
	reading := tableReading{tableBytes: 50 * 8192, liveRows: 0, fillfactor: 100, blockSize: 8192}
	if waste, known := reading.wasteBytes(); !known || waste != 50*8192 {
		t.Fatalf("waste = %d (known %v), want the whole file", waste, known)
	}
}

func TestATableIsListedOnlyOverBothThresholds(t *testing.T) {
	limits := Thresholds{MinWastePercent: 20, MinWasteBytes: 100}
	for _, tc := range []struct {
		name              string
		waste, tableBytes int64
		want              bool
	}{
		{"over both", 300, 1000, true},
		{"exactly on both", 200, 1000, true},
		{"share under", 199, 1000, false},
		{"bytes under", 99, 400, false},
	} {
		if got := limits.admit(tc.waste, tc.tableBytes); got != tc.want {
			t.Errorf("%s: admit(%d, %d) = %v, want %v", tc.name, tc.waste, tc.tableBytes, got, tc.want)
		}
	}
}

func TestTheStatementQuotesTheNames(t *testing.T) {
	reading := tableReading{schema: "public", table: `odd"name`, owner: "margince_owner", tableBytes: 1000}
	got := reading.recommendation(250)
	if got.Statement != `VACUUM (FULL, ANALYZE) "public"."odd""name";` {
		t.Fatalf("statement = %s", got.Statement)
	}
	if got.WastePercent != 25 {
		t.Fatalf("waste percent = %d, want 25", got.WastePercent)
	}
}
