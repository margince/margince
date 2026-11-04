// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package datasource

import (
	"cmp"
	"reflect"
	"strconv"
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
// A zero required number passes, as a zero time does in RejectOutOfRangeTimes.
// It is also what an omitted field decodes to.
//
//craft:ignore naked-any mirror of StrictDecode's seam target
func RejectOutOfBoundsNumbers(decoded any) *FieldShapeError {
	return taggedFieldRefusal(reflect.ValueOf(decoded), "", outOfBounds)
}

func outOfBounds(field reflect.StructField, value reflect.Value, name string) *FieldShapeError {
	low, hasLow := field.Tag.Lookup(MinimumTag)
	high, hasHigh := field.Tag.Lookup(MaximumTag)
	if !hasLow && !hasHigh {
		return nil
	}
	if value.Kind() != reflect.Pointer && value.IsZero() {
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
