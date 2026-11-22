// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package vacuumhealth

import "testing"

const page = 8192

var (
	boolean = column{align: 'c', length: 1, width: 1}
	bigint  = column{align: 'd', length: 8, width: 8}
)

func measured(rows float64, pages int64, columns ...column) tableReading {
	return tableReading{
		tableBytes: pages * page, liveRows: rows, fillfactor: 100, blockSize: page,
		attributes: len(columns), withStats: len(columns), columns: columns,
	}
}

// Four bool and bigint pairs store 64 bytes of values, not 36: each bigint
// starts on an 8-byte boundary. 92 bytes a row puts 88 rows in a page.
func TestAFreshlyLoadedTableWithPaddedColumnsHasNoWaste(t *testing.T) {
	reading := measured(8800, 100, boolean, bigint, boolean, bigint, boolean, bigint, boolean, bigint)
	if got := reading.rowBytes(); got != 92 {
		t.Fatalf("row = %d bytes, want 92", got)
	}
	if waste, known := reading.wasteBytes(); !known || waste != 0 {
		t.Fatalf("waste = %d (known %v), want none", waste, known)
	}
}

func TestANullableColumnAddsTheNullBitmapToTheHeader(t *testing.T) {
	columns := []column{bigint, bigint, bigint, bigint, bigint, bigint, bigint, bigint, bigint}
	without := measured(1, 1, columns...)
	nullable := append([]column{{align: 'd', length: 8, width: 8, nullFrac: 0.5}}, columns[1:]...)
	with := measured(1, 1, nullable...)
	// Nine columns need a 2-byte bitmap, which pushes the 23-byte header past 24.
	if got, want := with.rowBytes()-without.rowBytes(), int64(8); got != want {
		t.Fatalf("the bitmap adds %d bytes, want %d", got, want)
	}
}

// A short text value has a one-byte header and is not aligned.
func TestAShortTextValueIsStoredUnaligned(t *testing.T) {
	text := column{align: 'i', length: -1, width: 5}
	if got := measured(1, 1, boolean, text).rowBytes(); got != 24+8+4 {
		t.Fatalf("row = %d bytes, want %d", got, 24+8+4)
	}
}

// 1000 rows of a bigint and a 40-byte text take 76 bytes each, so 107 fit in a
// page and 10 pages hold them.
func TestTheLiveRowsLeaveTheRestOfTheFileAsWaste(t *testing.T) {
	reading := measured(1000, 100, bigint, column{align: 'i', length: -1, width: 40})
	if waste, known := reading.wasteBytes(); !known || waste != 90*page {
		t.Fatalf("waste = %d (known %v), want %d", waste, known, 90*page)
	}
}

func TestAFillfactorUnderAHundredIsSpaceTheTableAskedFor(t *testing.T) {
	full := measured(1000, 100, bigint, column{align: 'i', length: -1, width: 40})
	half := full
	half.fillfactor = 50
	fullWaste, _ := full.wasteBytes()
	halfWaste, _ := half.wasteBytes()
	if halfWaste >= fullWaste {
		t.Fatalf("fillfactor 50 leaves waste %d, want under the %d of a full packing", halfWaste, fullWaste)
	}
}

func TestATableTheStatisticsDoNotFullyDescribeHasNoEstimate(t *testing.T) {
	neverAnalyzed := measured(-1, 1, bigint)
	missingColumn := measured(10, 1, bigint, boolean)
	missingColumn.withStats = 1
	for name, reading := range map[string]tableReading{"never analyzed": neverAnalyzed, "a column without statistics": missingColumn} {
		if _, known := reading.wasteBytes(); known {
			t.Errorf("%s: an estimate was made", name)
		}
	}
}

// An analyzed table with every row deleted has no column statistics at all,
// and the whole file is waste.
func TestAnEmptiedTableIsAllWaste(t *testing.T) {
	reading := measured(0, 50, bigint)
	reading.withStats = 0
	if waste, known := reading.wasteBytes(); !known || waste != 50*page {
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

// With no thresholds at all, an empty file still has nothing to win back.
func TestAnEmptyFileIsNeverListed(t *testing.T) {
	if (Thresholds{}).admit(0, 0) {
		t.Fatal("an empty file is listed")
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
