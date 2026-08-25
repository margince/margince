// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import "reflect"

// ChangedColumns narrows a before/after pair to the columns that actually moved.
//
// A field-history projection reads every key in the pair, so carrying an
// untouched column through would publish "industry: Automotive → Automotive" as
// a change on a run that only filled the legal name.
//
// It reads the UNION of both images, not just the after one: a column the write
// emptied moves as surely as one it filled, and walking only `after` records no
// change at all for it. A column missing from either side reads as nil, which is
// what an absent value means in an audit image.
//
// Values compare by deep equality because a column holds whatever SQL type its
// table gives it — a `[]string` of social handles among them — and Go's `==`
// panics on the uncomparable ones rather than answering.
//
//craft:ignore naked-any column values span every SQL type a module owns; these are the schemaless audit images
func ChangedColumns(before, after map[string]any) (map[string]any, map[string]any) {
	changedBefore, changedAfter := map[string]any{}, map[string]any{}
	for _, column := range union(before, after) {
		oldValue, newValue := before[column], after[column]
		if sameColumnValue(oldValue, newValue) {
			continue
		}
		changedBefore[column] = oldValue
		changedAfter[column] = newValue
	}
	return changedBefore, changedAfter
}

//craft:ignore naked-any same column-value contract as ChangedColumns
func union(before, after map[string]any) []string {
	columns := make([]string, 0, len(before)+len(after))
	for column := range before {
		columns = append(columns, column)
	}
	for column := range after {
		if _, seen := before[column]; !seen {
			columns = append(columns, column)
		}
	}
	return columns
}

// sameColumnValue answers whether a column moved. A typed nil and an untyped one
// are the same absence: a writer that hands back an empty `[]string` where the
// row held SQL NULL has changed nothing, and reporting that as a change puts a
// field on the history screen that nobody edited.
//
//craft:ignore naked-any same column-value contract as ChangedColumns
func sameColumnValue(oldValue, newValue any) bool {
	if AbsentImage(oldValue) && AbsentImage(newValue) {
		return true
	}
	return reflect.DeepEqual(oldValue, newValue)
}
