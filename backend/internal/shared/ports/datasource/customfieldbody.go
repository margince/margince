// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package datasource

// A request body whose contract takes custom-field values as extra top-level
// cf_ keys, decoded strictly for every other key.
//
// The generated type for such a body carries an AdditionalProperties map and
// its own UnmarshalJSON. That method admits any key at any depth and splits a
// nested field's path, so it would accept a misspelt core field. So the cf_
// keys are lifted out first. The rest decodes into a twin of the struct with
// neither the map nor the method, by a closed contract struct's rules.

import (
	"encoding/json"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"
)

// CustomFieldPrefix starts every key a custom-field value travels under.
const CustomFieldPrefix = "cf_"

// IsCustomFieldKey reports whether key is spelt as a custom field's: the prefix
// and a name after it. It is the one test of "this key is not a typo of a core
// field", for the REST decode and the tool seam alike.
func IsCustomFieldKey(key string) bool {
	return len(key) > len(CustomFieldPrefix) && strings.HasPrefix(key, CustomFieldPrefix)
}

// CustomFieldBody is one body split into its core half and its cf_ values.
type CustomFieldBody struct {
	rest   json.RawMessage
	custom map[string]any
	twin   reflect.Value
	target reflect.Value
}

// SplitCustomFields lifts the cf_ keys out of raw, and readies a twin of into
// for the rest. into must point at a struct with an AdditionalProperties map.
// A body that is not a JSON object is left whole, for the decode to refuse.
//
//craft:ignore naked-any the deserialization seam: the target is whichever contract request struct the caller owns
func SplitCustomFields(raw json.RawMessage, into any) (*CustomFieldBody, error) {
	target := reflect.ValueOf(into)
	if target.Kind() != reflect.Pointer || target.Elem().Kind() != reflect.Struct {
		return nil, fmt.Errorf("datasource: %T is not a pointer to a request struct", into)
	}
	core, ok := coreFields(target.Elem().Type())
	if !ok {
		return nil, fmt.Errorf("datasource: %T carries no custom-field map", into)
	}
	body := &CustomFieldBody{rest: raw, twin: reflect.New(reflect.StructOf(core)), target: target.Elem()}
	keys, isObject := topLevelKeys(raw)
	if !isObject {
		return body, nil
	}
	for key, value := range keys {
		if !IsCustomFieldKey(key) {
			continue
		}
		var decoded any
		if err := json.Unmarshal(value, &decoded); err != nil {
			return nil, &FieldDecodeError{Cause: err}
		}
		if body.custom == nil {
			body.custom = map[string]any{}
		}
		body.custom[key] = decoded
		delete(keys, key)
	}
	if body.custom != nil {
		rest, err := json.Marshal(keys)
		if err != nil {
			return nil, err
		}
		body.rest = rest
	}
	return body, nil
}

// ClaimKeys offers each top-level key of raw to claim, in key order, and answers
// the first refusal. A layer that judges a key in its own words, such as a
// smuggled money total, claims it before the strict decode names it unknown.
func ClaimKeys(raw json.RawMessage, claim func(key string) error) error {
	keys, isObject := topLevelKeys(raw)
	if !isObject {
		return nil
	}
	for _, key := range slices.Sorted(maps.Keys(keys)) {
		if err := claim(key); err != nil {
			return err
		}
	}
	return nil
}

// Rest is the body without its cf_ keys.
func (b *CustomFieldBody) Rest() json.RawMessage { return b.rest }

// Target is the twin the rest decodes into.
//
//craft:ignore naked-any the twin is a struct type built at run time, so no static type names it
func (b *CustomFieldBody) Target() any { return b.twin.Interface() }

// Adopt copies the decoded twin into the caller's struct, with the cf_ values
// in its AdditionalProperties map.
func (b *CustomFieldBody) Adopt() {
	twin := b.twin.Elem()
	for i := range twin.NumField() {
		b.target.FieldByName(twin.Type().Field(i).Name).Set(twin.Field(i))
	}
	if b.custom != nil {
		b.target.FieldByName("AdditionalProperties").Set(reflect.ValueOf(b.custom))
	}
}

// StrictCustomFieldDecode is StrictDecode for a body that carries cf_ values.
//
//craft:ignore naked-any mirror of StrictDecode's seam target
func StrictCustomFieldDecode(raw json.RawMessage, into any) error {
	body, err := SplitCustomFields(raw, into)
	if err != nil {
		return err
	}
	if err := StrictDecode(body.Rest(), body.Target()); err != nil {
		return err
	}
	body.Adopt()
	return nil
}

// coreFields answers a struct's fields without its AdditionalProperties map,
// and whether it had one.
func coreFields(t reflect.Type) ([]reflect.StructField, bool) {
	var core []reflect.StructField
	found := false
	for field := range t.Fields() {
		if field.Name == "AdditionalProperties" && field.Type.Kind() == reflect.Map {
			found = true
			continue
		}
		core = append(core, field)
	}
	return core, found
}
