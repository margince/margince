// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package datasource

// The instants a request may store. Postgres keeps any timestamptz, but
// encoding/json refuses a time.Time outside years 0 to 9999. A stored instant
// is rendered in a zone up to 14 hours from UTC. A bound one day inside each
// end keeps every rendering in range, so no accepted value breaks a later read.

import (
	"reflect"
	"strconv"
	"strings"
	"time"
)

var (
	earliestAcceptedInstant = time.Date(1, time.January, 2, 0, 0, 0, 0, time.UTC)
	latestAcceptedInstant   = time.Date(9999, time.December, 31, 0, 0, 0, 0, time.UTC).Add(-time.Nanosecond)
	timeType                = reflect.TypeFor[time.Time]()
)

// acceptedInstantRange is the bound as the caller reads it, in the same words
// as the dates above.
const acceptedInstantRange = "a date-time from 0001-01-02 to 9999-12-30"

// RejectOutOfRangeTimes names the first decoded date-time a request may not
// store. Every decoder calls it on its target, so the bound holds for each
// timestamp field on every surface.
//
// A zero time.Time behind a pointer is refused, since the pointer says the
// caller sent it. A non-pointer zero passes: it is also what an omitted field
// decodes to, and the two cannot be told apart after the decode.
//
//craft:ignore naked-any mirror of StrictDecode's seam target
func RejectOutOfRangeTimes(decoded any) *FieldShapeError {
	var walk timeWalk
	if !walk.outOfRange(reflect.ValueOf(decoded), false) {
		return nil
	}
	return &FieldShapeError{Field: walk.path(), Want: acceptedInstantRange, Got: "a date-time outside that range"}
}

// timeWalk keeps the path to the value under inspection as a stack of
// segments. It renders the path only for a refusal, so each level of a deep
// body costs a push and a pop, not a new string.
type timeWalk struct{ segments []string }

func (w *timeWalk) outOfRange(value reflect.Value, sent bool) bool {
	if !value.IsValid() {
		return false
	}
	if value.Type() == timeType {
		return instantRefused(value, sent)
	}
	switch value.Kind() {
	case reflect.Pointer:
		return w.outOfRange(value.Elem(), true)
	case reflect.Interface:
		return w.outOfRange(value.Elem(), sent)
	case reflect.Struct:
		return w.outOfRangeField(value)
	case reflect.Slice, reflect.Array:
		for i := range value.Len() {
			if w.within("["+strconv.Itoa(i)+"]", value.Index(i)) {
				return true
			}
		}
	case reflect.Map:
		iter := value.MapRange()
		for iter.Next() {
			if w.within(iter.Key().String(), iter.Value()) {
				return true
			}
		}
	}
	return false
}

// instantRefused judges one time.Time; sent says a pointer carried it.
func instantRefused(value reflect.Value, sent bool) bool {
	instant, ok := reflect.TypeAssert[time.Time](value)
	if !ok || (instant.IsZero() && !sent) {
		return false
	}
	return instant.Before(earliestAcceptedInstant) || instant.After(latestAcceptedInstant)
}

func (w *timeWalk) outOfRangeField(value reflect.Value) bool {
	for i := range value.NumField() {
		described := value.Type().Field(i)
		if !described.IsExported() {
			continue
		}
		name := wireName(described)
		if described.Anonymous || name == "" {
			if w.outOfRange(value.Field(i), false) {
				return true
			}
			continue
		}
		if w.within(name, value.Field(i)) {
			return true
		}
	}
	return false
}

// within inspects value one segment deeper, and leaves the segment on the
// stack when it is the refused one.
func (w *timeWalk) within(segment string, value reflect.Value) bool {
	w.segments = append(w.segments, segment)
	if w.outOfRange(value, false) {
		return true
	}
	w.segments = w.segments[:len(w.segments)-1]
	return false
}

func (w *timeWalk) path() string {
	var b strings.Builder
	for i, segment := range w.segments {
		if i > 0 && !strings.HasPrefix(segment, "[") {
			b.WriteByte('.')
		}
		b.WriteString(segment)
	}
	return b.String()
}

// wireName is the key a caller sent for this field: its json tag, or the Go
// name encoding/json falls back to. A `json:"-"` field answers "": it is a
// generated additionalProperties map, whose own keys are the wire keys.
func wireName(field reflect.StructField) string {
	name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
	switch name {
	case "-":
		return ""
	case "":
		return field.Name
	}
	return name
}
