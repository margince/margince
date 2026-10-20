// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"bytes"
	"encoding/json"
	"reflect"
	"time"
)

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
// Two values are also the same when the audit image renders them alike. A
// stored *string equals the string a request carries. The undo path compares an
// image with the live row the same way.
//
//craft:ignore naked-any same column-value contract as ChangedColumns
func sameColumnValue(oldValue, newValue any) bool {
	if emptyColumnValue(oldValue) && emptyColumnValue(newValue) {
		return true
	}
	if reflect.DeepEqual(oldValue, newValue) {
		return true
	}
	if oldAt, newAt, bothTimes := instants(oldValue, newValue); bothTimes {
		return oldAt.Equal(newAt)
	}
	oldJSON, oldErr := json.Marshal(oldValue)
	newJSON, newErr := json.Marshal(newValue)
	return oldErr == nil && newErr == nil && bytes.Equal(oldJSON, newJSON)
}

// instants reads two timestamps, by value or by pointer, as moments. One instant
// in two zones is one value to the column.
//
//craft:ignore naked-any same column-value contract as ChangedColumns
func instants(a, b any) (time.Time, time.Time, bool) {
	first, firstOK := instantOf(a)
	second, secondOK := instantOf(b)
	return first, second, firstOK && secondOK
}

//craft:ignore naked-any same column-value contract as ChangedColumns
func instantOf(v any) (time.Time, bool) {
	switch at := v.(type) {
	case time.Time:
		return at, true
	case *time.Time:
		if at != nil {
			return *at, true
		}
	}
	return time.Time{}, false
}

// emptyColumnValue reads a column that holds nothing. A nil slice and an empty
// one are one state to the column and to every reader of the image, and
// reflect.DeepEqual separates them — which would publish a writer that handed
// back an empty list where the row held none as a change nobody made.
//
//craft:ignore naked-any same column-value contract as ChangedColumns
func emptyColumnValue(v any) bool {
	if AbsentImage(v) {
		return true
	}
	switch rv := reflect.ValueOf(v); rv.Kind() {
	case reflect.Slice, reflect.Map:
		return rv.Len() == 0
	default:
		return false
	}
}
