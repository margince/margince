// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package datasource

import (
	"reflect"
	"unicode"
	"unicode/utf8"
)

// SingleLineTag is the struct tag the contract generator puts on every text
// field whose schema says `x-single-line: true`. Its value is always "true".
const SingleLineTag = "singleline"

// RejectUnsafeSingleLine names the first single-line text field that carries a
// control character, a line break or a bidirectional override. Every decoder
// calls it on its target, so a name is held to one rule on every surface.
//
//craft:ignore naked-any mirror of StrictDecode's seam target
func RejectUnsafeSingleLine(decoded any) *FieldShapeError {
	return taggedFieldRefusal(reflect.ValueOf(decoded), "", func(field reflect.StructField, value reflect.Value, name string) *FieldShapeError {
		if field.Tag.Get(SingleLineTag) != "true" {
			return nil
		}
		for value.Kind() == reflect.Pointer && !value.IsNil() {
			value = value.Elem()
		}
		if value.Kind() != reflect.String || !hasUnsafeSingleLineRune(value.String()) {
			return nil
		}
		return &FieldShapeError{Field: name, Want: "text on one line, without control characters or direction overrides"}
	})
}

func hasUnsafeSingleLineRune(text string) bool {
	for _, r := range text {
		if unicode.IsControl(r) || r == '\u2028' || r == '\u2029' ||
			(r >= '\u202a' && r <= '\u202e') || (r >= '\u2066' && r <= '\u2069') {
			return true
		}
	}
	return false
}

// HasMalformedText reports a JSON body that is not valid UTF-8, or that spells
// a lone surrogate as a `\uD800` escape. encoding/json would store either as
// U+FFFD, changing the sender's text with no sign of it.
func HasMalformedText(raw []byte) bool {
	if !utf8.Valid(raw) {
		return true
	}
	for i := 0; i+1 < len(raw); i++ {
		if raw[i] != '\\' {
			continue
		}
		if raw[i+1] != 'u' {
			i++ // skips the escaped character, so `\\u` is a backslash and a u
			continue
		}
		unit, ok := hexUnit(raw, i+2)
		if !ok {
			continue
		}
		switch {
		case unit >= 0xD800 && unit <= 0xDBFF:
			low, paired := pairedLow(raw, i+6)
			if !paired || low < 0xDC00 || low > 0xDFFF {
				return true
			}
			i += 11
		case unit >= 0xDC00 && unit <= 0xDFFF:
			return true
		default:
			i += 5
		}
	}
	return false
}

// pairedLow reads the `\uXXXX` that must follow a high surrogate at i.
func pairedLow(raw []byte, i int) (rune, bool) {
	if i+1 >= len(raw) || raw[i] != '\\' || raw[i+1] != 'u' {
		return 0, false
	}
	return hexUnit(raw, i+2)
}

func hexUnit(raw []byte, i int) (rune, bool) {
	if i+4 > len(raw) {
		return 0, false
	}
	var unit rune
	for _, c := range raw[i : i+4] {
		switch {
		case c >= '0' && c <= '9':
			unit = unit<<4 | rune(c-'0')
		case c >= 'a' && c <= 'f':
			unit = unit<<4 | rune(c-'a'+10)
		case c >= 'A' && c <= 'F':
			unit = unit<<4 | rune(c-'A'+10)
		default:
			return 0, false
		}
	}
	return unit, true
}
