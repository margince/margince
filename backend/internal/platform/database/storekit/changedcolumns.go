// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"bytes"
	"encoding/json"
	"reflect"
	"time"
)

// ChangedColumns keeps audit images from reporting unchanged fields.
// Missing and empty values represent the same absence.
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
// stored *string equals the string a request carries. A `date` column goes
// through Patch.SetDate, which renders both sides as text, so this fallback
// never sees one as a time.Time.
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
		return SameStoredInstant(oldAt, newAt)
	}
	oldJSON, oldErr := json.Marshal(oldValue)
	newJSON, newErr := json.Marshal(newValue)
	return oldErr == nil && newErr == nil && bytes.Equal(oldJSON, newJSON)
}

// SameStoredInstant compares timestamps at PostgreSQL's microsecond precision.
func SameStoredInstant(a, b time.Time) bool {
	return a.Truncate(time.Microsecond).Equal(b.Truncate(time.Microsecond))
}

// instants reads two timestamps, by value or by pointer, as moments. One instant
// in two zones is one value to the column. The compose layer's sameInstant
// parses jsonb text before using the same stored-time comparison.
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
