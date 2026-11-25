// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package datasource

// A request rule the generated email type does not hold on decode: an address
// is sent as user@domain.

import (
	"encoding/json"
	"net/mail"
	"reflect"
	"strconv"
	"strings"
)

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
		if decodeErr := json.Unmarshal(raw, &items); decodeErr != nil {
			return nil //nolint:nilerr // a value that is no array is the decoder's to refuse
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
	if decodeErr := json.Unmarshal(raw, &text); decodeErr != nil {
		return nil //nolint:nilerr // a value that is no string is the decoder's to refuse
	}
	parsed, parseErr := mail.ParseAddress(text)
	if parseErr != nil {
		return nil //nolint:nilerr // an address that does not parse is the decoder's to refuse
	}
	if parsed.Address == strings.TrimSpace(text) {
		return nil
	}
	return &FieldShapeError{
		Field: path, Want: "a plain email address (user@domain)",
		Got: "an address with a display name or angle brackets", code: "email_malformed",
	}
}
