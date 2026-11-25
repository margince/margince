// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package datasource

// Two request rules the generated types do not hold on decode: an enum value
// is one the contract lists, and an email address is sent as the bare address.

import (
	"encoding/json"
	"net/mail"
	"reflect"
	"strconv"
	"strings"

	"github.com/margince/margince/backend/internal/shared/kernel/textcut"
)

// RejectUnknownEnumValues names the first enum value sent that its field does
// not list. A generated enum is a string type with a Valid method, and its
// decode accepts any string, so a value outside the list reaches a CHECK and
// is refused there without a field. owned names the paths a later layer judges.
//
//craft:ignore naked-any mirror of StrictDecode's seam target
func RejectUnknownEnumValues(decoded any, raw json.RawMessage, owned func(path string) bool) *FieldShapeError {
	sent := sentKeys{raw: raw}
	return taggedFieldRefusal(reflect.ValueOf(decoded), "", func(_ reflect.StructField, value reflect.Value, name string) *FieldShapeError {
		if owned != nil && owned(name) {
			return nil
		}
		return unknownEnumValue(value, name, &sent)
	})
}

func unknownEnumValue(value reflect.Value, name string, sent *sentKeys) *FieldShapeError {
	if value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return nil
		}
		value = value.Elem()
	} else if value.IsZero() && !sent.holds(name) {
		return nil
	}
	if value.Kind() == reflect.Slice {
		for i := range value.Len() {
			if refused := unknownEnumValue(value.Index(i), name+"["+strconv.Itoa(i)+"]", sent); refused != nil {
				return refused
			}
		}
		return nil
	}
	if value.Kind() != reflect.String || listsValue(value) {
		return nil
	}
	return &FieldShapeError{
		Field: name, Want: "one of the values this field's schema lists",
		Got: strconv.Quote(textcut.BytesMarked(value.String(), maxEchoedValue, "…")), code: "invalid_enum",
	}
}

// listsValue answers the generated Valid method, and true for a string type
// that has none, which is no enum.
func listsValue(value reflect.Value) bool {
	valid := value.MethodByName("Valid")
	if !valid.IsValid() || valid.Type().NumIn() != 0 || valid.Type().NumOut() != 1 || valid.Type().Out(0).Kind() != reflect.Bool {
		return true
	}
	return valid.Call(nil)[0].Bool()
}

// maxEchoedValue bounds a caller's own value quoted back in a refusal.
const maxEchoedValue = 64

// emailTypeName is the name the contract generator gives its email format.
const emailTypeName = "Email"

// RejectDisplayNameEmails names the first address sent in a form other than
// user@domain, such as `Name <x@example.com>`. The generated email type keeps
// only the address, so such a value would be stored as text nobody sent.
//
//craft:ignore naked-any mirror of StrictDecode's seam target
func RejectDisplayNameEmails(decoded any, raw json.RawMessage) *FieldShapeError {
	return displayNameEmail(raw, reflect.TypeOf(decoded), "")
}

// displayNameEmail walks raw beside the type it decoded into. Each step goes
// one level into raw, so the walk ends with the body however the type nests.
func displayNameEmail(raw json.RawMessage, t reflect.Type, path string) *FieldShapeError {
	t = derefType(t)
	switch {
	case t == nil:
		return nil
	case t.Kind() == reflect.String && t.Name() == emailTypeName:
		return emailWithDisplayName(raw, path)
	case t.Kind() == reflect.Struct:
		keys, isObject := topLevelKeys(raw)
		if !isObject {
			return nil
		}
		for _, key := range declaredFieldKeys(t, keys) {
			field, _ := fieldByJSONName(t, key)
			if refused := displayNameEmail(keys[key], field.Type, joinWirePath(path, key)); refused != nil {
				return refused
			}
		}
	case t.Kind() == reflect.Slice:
		var items []json.RawMessage
		if json.Unmarshal(raw, &items) != nil {
			return nil
		}
		for i, item := range items {
			if refused := displayNameEmail(item, t.Elem(), path+"["+strconv.Itoa(i)+"]"); refused != nil {
				return refused
			}
		}
	}
	return nil
}

func emailWithDisplayName(raw json.RawMessage, path string) *FieldShapeError {
	var text string
	if json.Unmarshal(raw, &text) != nil {
		return nil
	}
	parsed, err := mail.ParseAddress(text)
	if err != nil || parsed.Address == strings.TrimSpace(text) {
		return nil
	}
	return &FieldShapeError{
		Field: path, Want: "a plain email address (user@domain)",
		Got: "an address with a display name or angle brackets", code: "email_malformed",
	}
}
