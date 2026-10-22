// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package httperr

import "reflect"

// withEmptyLists returns body with a nil Data slice replaced by an empty one, so
// a list with no rows goes out as `"data": []` and not `"data": null`.
//
// The contract types `data` as an array, and `null` is not one. A nil slice is
// the idiomatic empty list in Go, so handlers return it, and only encoding/json
// sees the difference. The generated TypeScript trusts the contract, so
// `data.data.map(...)` threw on such a response and took the screen down.
//
// It is fixed here because WriteJSON is the one door every list envelope leaves
// by. A guard in each reader or handler would be a rule someone has to remember.
//
// It works on a copy. Handlers write struct values, which are not addressable
// through an `any`, so setting the field in place would do nothing for them.
//
//craft:ignore naked-any the response-body seam: it takes whatever envelope a handler wrote, which is the same any WriteJSON takes
func withEmptyLists(body any) any {
	value := reflect.ValueOf(body)
	// Through a pointer the field is settable in place, and the caller holds
	// the same struct either way, so there is nothing to copy.
	if value.Kind() == reflect.Pointer && !value.IsNil() {
		if data := nilDataField(value.Elem()); data.IsValid() && data.CanSet() {
			data.Set(reflect.MakeSlice(data.Type(), 0, 0))
		}
		return body
	}
	if !nilDataField(value).IsValid() {
		return body
	}
	// An addressable copy, so the field this one carries can be set.
	fixed := reflect.New(value.Type()).Elem()
	fixed.Set(value)
	data := fixed.FieldByName("Data")
	data.Set(reflect.MakeSlice(data.Type(), 0, 0))
	return fixed.Interface()
}

// nilDataField is the struct's `Data` field when it is a nil slice, and the
// zero Value otherwise. It is the one condition this normalisation acts on.
func nilDataField(value reflect.Value) reflect.Value {
	if value.Kind() != reflect.Struct {
		return reflect.Value{}
	}
	data := value.FieldByName("Data")
	if !data.IsValid() || data.Kind() != reflect.Slice || !data.IsNil() {
		return reflect.Value{}
	}
	return data
}
