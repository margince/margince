// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package datasource

import (
	"fmt"
	"reflect"
	"strconv"
)

// UniqueItemsTag is the struct tag the contract generator puts on every field
// whose schema says `uniqueItems: true`. Its value is always "items".
const UniqueItemsTag = "unique"

// RejectRepeatedItems names the first `uniqueItems` list that the request sent
// with one item twice. Every decoder calls it on its target, as it calls
// RejectOutOfRangeTimes. So the rule holds on every surface with no check in a
// handler.
//
//craft:ignore naked-any mirror of StrictDecode's seam target
func RejectRepeatedItems(decoded any) *FieldShapeError {
	return repeatedWithin(reflect.ValueOf(decoded), "")
}

func repeatedWithin(value reflect.Value, path string) *FieldShapeError {
	switch value.Kind() {
	case reflect.Pointer, reflect.Interface:
		if value.IsNil() {
			return nil
		}
		return repeatedWithin(value.Elem(), path)
	case reflect.Struct:
		return repeatedInFields(value, path)
	case reflect.Slice, reflect.Array:
		for i := range value.Len() {
			if refused := repeatedWithin(value.Index(i), path+"["+strconv.Itoa(i)+"]"); refused != nil {
				return refused
			}
		}
	case reflect.Map:
		iter := value.MapRange()
		for iter.Next() {
			if refused := repeatedWithin(iter.Value(), joinWirePath(path, iter.Key().String())); refused != nil {
				return refused
			}
		}
	}
	return nil
}

func repeatedInFields(value reflect.Value, path string) *FieldShapeError {
	for i := range value.NumField() {
		described := value.Type().Field(i)
		if !described.IsExported() {
			continue
		}
		name := path
		if wire := wireName(described); !described.Anonymous && wire != "" {
			name = joinWirePath(path, wire)
		}
		if described.Tag.Get(UniqueItemsTag) == "items" {
			if refused := repeatedItem(value.Field(i), name); refused != nil {
				return refused
			}
		}
		if refused := repeatedWithin(value.Field(i), name); refused != nil {
			return refused
		}
	}
	return nil
}

// repeatedItem refuses a list holding one item twice, naming both positions.
// Generated item types are strings, UUIDs and enums, which a map can key; an
// item that is not comparable falls back to a deep comparison.
func repeatedItem(list reflect.Value, field string) *FieldShapeError {
	for list.Kind() == reflect.Pointer && !list.IsNil() {
		list = list.Elem()
	}
	if list.Kind() != reflect.Slice {
		return nil
	}
	seen := make(map[any]int, list.Len())
	for i := range list.Len() {
		item := list.Index(i)
		first, found := firstEqual(list, item, i, seen)
		if found {
			return &FieldShapeError{
				Field: field,
				Want:  "a list that names each item once",
				Got:   fmt.Sprintf("a list whose item [%d] repeats item [%d]", i, first),
			}
		}
	}
	return nil
}

func firstEqual(list, item reflect.Value, at int, seen map[any]int) (int, bool) {
	if item.Comparable() {
		key := item.Interface()
		if first, found := seen[key]; found {
			return first, true
		}
		seen[key] = at
		return 0, false
	}
	for earlier := range at {
		if reflect.DeepEqual(list.Index(earlier).Interface(), item.Interface()) {
			return earlier, true
		}
	}
	return 0, false
}

func joinWirePath(path, segment string) string {
	if path == "" {
		return segment
	}
	return path + "." + segment
}
