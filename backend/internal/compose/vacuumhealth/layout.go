// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package vacuumhealth

import "math"

// tableReading is one table as the catalog describes it after its last
// VACUUM or ANALYZE.
type tableReading struct {
	schema, table, owner string
	// tableBytes is relpages times the block size, from the same run as liveRows.
	tableBytes int64
	// liveRows is the planner's row count; below zero, nothing measured the table yet.
	liveRows   float64
	fillfactor int
	blockSize  int
	// attributes counts every column a row header covers, dropped ones included.
	attributes int
	// withStats counts the live columns pg_stats describes.
	withStats int
	// columns are the live columns in storage order.
	columns []column
}

type column struct {
	// align is pg_attribute.attalign: c, s, i or d.
	align  byte
	length int
	// width is pg_stats.avg_width, the stored size of a value that is not null.
	width    float64
	nullFrac float64
}

// The on-disk sizes of PostgreSQL's heap. A row has a header before its null
// bitmap and is aligned as a whole; a page has a header and a pointer per row.
const (
	rowHeaderBytes  = 23
	maxAlignBytes   = 8
	pageHeaderBytes = 24
	rowPointerBytes = 4
	// A text value under this size has a one-byte header and is stored unaligned.
	shortValueBytes = 127
)

// wasteBytes estimates the part of the file that holds no live row. That is the
// file less the pages the live rows fill when packed to the fillfactor.
// It is unknown when the statistics do not describe every live column.
func (t tableReading) wasteBytes() (int64, bool) {
	if t.liveRows < 0 {
		return 0, false
	}
	var needed int64
	if t.liveRows > 0 {
		if t.withStats != len(t.columns) {
			return 0, false
		}
		usable := int64(t.blockSize-pageHeaderBytes) * int64(t.fillfactor) / 100
		mostPerPage := int64(t.blockSize-pageHeaderBytes) / (alignUp(rowHeaderBytes, maxAlignBytes) + rowPointerBytes)
		perPage := min(max(usable/t.rowBytes(), 1), mostPerPage)
		needed = int64(math.Ceil(t.liveRows/float64(perPage))) * int64(t.blockSize)
	}
	return max(t.tableBytes-needed, 0), true
}

// rowBytes lays one average row out the way the heap stores it. That is the
// header and null bitmap, then each value at its type's alignment, padded to 8
// bytes, plus the row's pointer in its page.
func (t tableReading) rowBytes() int64 {
	header := int64(rowHeaderBytes)
	if t.mayHoldNulls() {
		header += int64(t.attributes+7) / 8
	}
	var data float64
	for _, col := range t.columns {
		if col.nullFrac >= 1 {
			continue
		}
		align := col.alignBytes()
		data = math.Ceil(data/align) * align
		data += max(col.width, float64(col.length)) * (1 - col.nullFrac)
	}
	return alignUp(alignUp(header, maxAlignBytes)+int64(math.Ceil(data)), maxAlignBytes) + rowPointerBytes
}

// mayHoldNulls is true when some row carries a null bitmap. A dropped column
// reads as null in every row written after the drop.
func (t tableReading) mayHoldNulls() bool {
	if t.attributes > len(t.columns) {
		return true
	}
	for _, col := range t.columns {
		if col.nullFrac > 0 {
			return true
		}
	}
	return false
}

func (c column) alignBytes() float64 {
	if c.length == -1 && c.width < shortValueBytes {
		return 1
	}
	switch c.align {
	case 's':
		return 2
	case 'i':
		return 4
	case 'd':
		return 8
	default:
		return 1
	}
}

func alignUp(n, to int64) int64 { return (n + to - 1) / to * to }
