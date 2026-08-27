// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package httperr

// How many records one response hands over, and the seam that lets the surface
// bounding an agent's reads charge for them.

import "reflect"

// ServedMeter is implemented by a ResponseWriter its owner has wrapped to count
// what leaves this door. WriteJSON reports every body it is about to write, so
// the count is taken at the ONE place a record becomes a REST response instead
// of in the ~290 handlers that would each have to remember.
//
// This package holds no request and knows nothing about quotas, so a meter that
// cannot record its charge answers the request ITSELF and reports proceed=false;
// WriteJSON then writes nothing more. That keeps the decision about what an
// uncountable answer costs with the door that has the context to make it.
type ServedMeter interface {
	NoteServed(n int) (proceed bool)
}

// recordsIn counts the records a response body hands over.
//
// It reads the CONTRACT's own list envelope rather than a list of response type
// names: every generated list response is a struct carrying a `Data` slice, so
// the shape answers the question and a list added tomorrow is counted without an
// edit here. TestEveryListResponseCarriesADataSlice holds that shape, so a list
// this rule would silently count as one fails the build instead.
//
// Anything else is one thing handed over, and that deliberately counts a
// settings or status body as a record: it is still an answer this credential was
// given, and separating "real" records from the rest is the maintained list
// again — with the tool added next missing from it.
//
//craft:ignore naked-any it counts WriteJSON's own body argument, and takes the same type that seam does
func recordsIn(body any) int {
	value := reflect.ValueOf(body)
	for value.Kind() == reflect.Pointer || value.Kind() == reflect.Interface {
		if value.IsNil() {
			return 0
		}
		value = value.Elem()
	}
	switch value.Kind() {
	case reflect.Invalid:
		// A nil body writes JSON "null" and hands over nothing.
		return 0
	case reflect.Slice:
		return value.Len()
	case reflect.Struct:
		if data := value.FieldByName("Data"); data.IsValid() && data.Kind() == reflect.Slice {
			return data.Len()
		}
		return 1
	default:
		return 1
	}
}

// withEmptyLists returns body with a nil Data slice replaced by an empty one,
// so a list with no rows goes out as `"data": []` rather than `"data": null`.
//
// The contract says every list envelope is `required: [data, page]` with `data`
// of `type: array`, and `null` is not an array — so a handler that returned a
// nil slice was violating its own contract on the wire. Go makes that the EASY
// mistake rather than an unusual one: a nil slice IS the idiomatic empty list,
// `append` to it works, `len` and `range` work, and the only place it behaves
// differently is `encoding/json`.
//
// The cost fell on every client. The generated TypeScript reads `data: Person[]`
// — because the contract says so — and gives a caller no reason to guard, so
// `data.data.map(...)` on such a response threw and took down the screen
// rendering it (issue #1606). Thirty-six reads across eighteen files were one
// thin response away from that, and no compiler could have told them.
//
// Fixed HERE rather than at those thirty-six, and rather than at each handler:
// WriteJSON is the one door every list envelope leaves by, and it already
// reflects for this exact field to count rows. A guard at the readers would
// have taught thirty-six call sites to expect a shape the contract forbids; a
// guard at each handler is a rule somebody has to remember. This is the only
// place that can be wrong once.
//
// A COPY, not a mutation of the caller's value. Every handler in this tree
// writes a struct VALUE — `crmcontracts.PersonListResponse{Data: people, …}` —
// and a value handed to an `any` parameter is not addressable, so a version of
// this that set the field in place did nothing at all for the case that
// actually ships. It passed its own unit test through a pointer.
//
//craft:ignore naked-any the response-body seam: it takes whatever envelope a handler wrote, which is the same any WriteJSON above it takes
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
// zero Value otherwise — the one condition this normalisation acts on.
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
