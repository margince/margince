// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package datasource

import (
	"cmp"
	"encoding/json"
	"reflect"
	"strconv"
	"strings"
)

// MinimumTag and MaximumTag are the struct tags the contract generator puts on
// a number whose schema declares `minimum` or `maximum`. Each value is the
// bound as the contract spells it.
const (
	MinimumTag = "minimum"
	MaximumTag = "maximum"
)

// RejectOutOfBoundsNumbers names the first number sent outside its declared
// range. Every decoder calls it, so no handler repeats a contract bound.
//
// raw is the body the value was decoded from. A zero with no pointer is judged
// only when raw holds its key. An omitted field decodes to the same zero, and
// the handler refuses a required field the caller left out.
//
// owned names the paths a later layer judges. That layer reports every
// problem of the body in one answer, which a refusal here would cut short.
//
//craft:ignore naked-any mirror of StrictDecode's seam target
func RejectOutOfBoundsNumbers(decoded any, raw json.RawMessage, owned func(path string) bool) *FieldShapeError {
	sent := sentKeys{raw: raw}
	return taggedFieldRefusal(reflect.ValueOf(decoded), "", func(field reflect.StructField, value reflect.Value, name string) *FieldShapeError {
		if owned != nil && owned(name) {
			return nil
		}
		return outOfBounds(field, value, name, &sent)
	})
}

func outOfBounds(field reflect.StructField, value reflect.Value, name string, sent *sentKeys) *FieldShapeError {
	low, hasLow := field.Tag.Lookup(MinimumTag)
	high, hasHigh := field.Tag.Lookup(MaximumTag)
	if !hasLow && !hasHigh {
		return nil
	}
	if value.Kind() != reflect.Pointer && value.IsZero() && !sent.holds(name) {
		return nil
	}
	for value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return nil
		}
		value = value.Elem()
	}
	underLow := hasLow && compareToBound(value, low) < 0
	overHigh := hasHigh && compareToBound(value, high) > 0
	if !underLow && !overHigh {
		return nil
	}
	return &FieldShapeError{Field: name, Want: boundsWanted(field.Tag), Got: numberText(value)}
}

// compareToBound orders a decoded number against a bound. A whole bound is
// compared to an integer as an integer, so a value past 2^53 cannot round onto it.
// A value that is no number answers 0: the decode already refused a wrong type,
// and the contract downgrade refuses a bound that is no number.
func compareToBound(value reflect.Value, bound string) int {
	switch {
	case value.CanInt():
		if whole, err := strconv.ParseInt(bound, 10, 64); err == nil {
			return cmp.Compare(value.Int(), whole)
		}
		return compareFloat(float64(value.Int()), bound)
	case value.CanUint():
		if whole, err := strconv.ParseUint(bound, 10, 64); err == nil {
			return cmp.Compare(value.Uint(), whole)
		}
		return compareFloat(float64(value.Uint()), bound)
	case value.CanFloat():
		return compareFloat(value.Float(), bound)
	}
	return 0
}

func compareFloat(number float64, bound string) int {
	limit, err := strconv.ParseFloat(bound, 64)
	if err != nil {
		return 0
	}
	return cmp.Compare(number, limit)
}

func boundsWanted(tag reflect.StructTag) string {
	low, hasLow := tag.Lookup(MinimumTag)
	high, hasHigh := tag.Lookup(MaximumTag)
	switch {
	case hasLow && hasHigh:
		return "a number from " + low + " to " + high
	case hasLow:
		return "a number of at least " + low
	}
	return "a number of at most " + high
}

func numberText(value reflect.Value) string {
	switch {
	case value.CanInt():
		return strconv.FormatInt(value.Int(), 10)
	case value.CanUint():
		return strconv.FormatUint(value.Uint(), 10)
	}
	return strconv.FormatFloat(value.Float(), 'g', -1, 64)
}

// sentKeys answers whether a body carried a value at a wire path such as
// `items[1].weight`. The body is read once, on the first zero that asks.
type sentKeys struct {
	raw  json.RawMessage
	tree any
	read bool
}

func (k *sentKeys) holds(path string) bool {
	if !k.read {
		k.read = true
		if len(k.raw) > 0 && json.Unmarshal(k.raw, &k.tree) != nil {
			k.tree = nil
		}
	}
	node := k.tree
	for segment := range strings.SplitSeq(strings.ReplaceAll(path, "[", ".["), ".") {
		if segment == "" {
			continue
		}
		var found bool
		if node, found = childAt(node, segment); !found {
			return false
		}
	}
	return node != nil
}

//craft:ignore naked-any a decoded JSON tree is maps, slices and scalars by construction
func childAt(node any, segment string) (any, bool) {
	if index, ok := strings.CutPrefix(segment, "["); ok {
		list, isList := node.([]any)
		at, err := strconv.Atoi(strings.TrimSuffix(index, "]"))
		if !isList || err != nil || at < 0 || at >= len(list) {
			return nil, false
		}
		return list[at], true
	}
	object, isObject := node.(map[string]any)
	if !isObject {
		return nil, false
	}
	child, found := object[segment]
	return child, found
}
