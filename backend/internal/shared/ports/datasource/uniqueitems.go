// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package datasource

import (
	"encoding/json"
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
	return taggedFieldRefusal(reflect.ValueOf(decoded), "", func(field reflect.StructField, value reflect.Value, name string) *FieldShapeError {
		if field.Tag.Get(UniqueItemsTag) != "items" {
			return nil
		}
		return repeatedItem(value, name)
	})
}

// fieldCheck judges one struct field of a decoded body; name is its wire path.
type fieldCheck func(field reflect.StructField, value reflect.Value, name string) *FieldShapeError

// taggedFieldRefusal runs check on every struct field reachable from value and
// answers its first refusal. The contract rules a decode cannot see travel as
// struct tags, so each rule is one check over this one walk.
func taggedFieldRefusal(value reflect.Value, path string, check fieldCheck) *FieldShapeError {
	switch value.Kind() {
	case reflect.Pointer, reflect.Interface:
		if value.IsNil() {
			return nil
		}
		return taggedFieldRefusal(value.Elem(), path, check)
	case reflect.Struct:
		return fieldsRefusal(value, path, check)
	case reflect.Slice, reflect.Array:
		for i := range value.Len() {
			if refused := taggedFieldRefusal(value.Index(i), path+"["+strconv.Itoa(i)+"]", check); refused != nil {
				return refused
			}
		}
	case reflect.Map:
		iter := value.MapRange()
		for iter.Next() {
			if refused := taggedFieldRefusal(iter.Value(), joinWirePath(path, iter.Key().String()), check); refused != nil {
				return refused
			}
		}
	}
	return nil
}

func fieldsRefusal(value reflect.Value, path string, check fieldCheck) *FieldShapeError {
	for i := range value.NumField() {
		described := value.Type().Field(i)
		if !described.IsExported() {
			continue
		}
		name := path
		if wire := wireName(described); !described.Anonymous && wire != "" {
			name = joinWirePath(path, wire)
		}
		if refused := check(described, value.Field(i), name); refused != nil {
			return refused
		}
		if refused := taggedFieldRefusal(value.Field(i), name, check); refused != nil {
			return refused
		}
	}
	return nil
}

// repeatedItem refuses a list holding one item twice, naming both positions.
// Items are compared as JSON, the equality `uniqueItems` is defined by, so a
// pointer or struct item counts by its value and never by its address.
func repeatedItem(list reflect.Value, field string) *FieldShapeError {
	for list.Kind() == reflect.Pointer && !list.IsNil() {
		list = list.Elem()
	}
	if list.Kind() != reflect.Slice {
		return nil
	}
	seen := make(map[string]int, list.Len())
	for i := range list.Len() {
		first, found := firstEqual(list, i, seen)
		if found {
			return RepeatedItemRefusal(field, first, i)
		}
	}
	return nil
}

// RepeatedItemRefusal is the one wording of a repeated item, for every decoder
// that finds one.
func RepeatedItemRefusal(field string, first, at int) *FieldShapeError {
	return &FieldShapeError{
		Field: field,
		Want:  "a list that names each item once",
		Got:   fmt.Sprintf("a list whose item [%d] repeats item [%d]", at, first),
	}
}

func firstEqual(list reflect.Value, at int, seen map[string]int) (int, bool) {
	item := list.Index(at).Interface()
	wire, err := json.Marshal(item)
	if err != nil {
		// An item whose own MarshalJSON fails has no wire form to compare, so
		// it is compared by value against every earlier item instead.
		for earlier := range at {
			if reflect.DeepEqual(list.Index(earlier).Interface(), item) {
				return earlier, true
			}
		}
		return 0, false
	}
	if first, found := seen[string(wire)]; found {
		return first, true
	}
	seen[string(wire)] = at
	return 0, false
}

func joinWirePath(path, segment string) string {
	if path == "" {
		return segment
	}
	return path + "." + segment
}
