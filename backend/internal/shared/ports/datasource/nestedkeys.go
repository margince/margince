// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package datasource

import (
	"encoding/json"
	"reflect"
	"slices"
	"strconv"
)

var unmarshalerType = reflect.TypeFor[json.Unmarshaler]()

// RejectUnknownNestedKeys refuses a key no struct on the way down declares,
// naming it by its path (`upstream.zdr`): encoding/json drops it, so the write
// would answer 200 for a setting it never stored. A type with its own
// UnmarshalJSON or a catch-all map owns its key policy and is not entered, and
// neither is a path `owned` claims for a layer that judges it itself.
//
//craft:ignore naked-any mirror of RejectNonCanonicalKeys's seam target
func RejectUnknownNestedKeys(raw json.RawMessage, into any, owned func(path string) bool) error {
	var unknown []string
	collectUnknownKeys(reflect.TypeOf(into), raw, "", owned, &unknown)
	if len(unknown) == 0 {
		return nil
	}
	slices.Sort(unknown)
	return &UnknownFieldError{Fields: unknown}
}

func collectUnknownKeys(t reflect.Type, raw json.RawMessage, path string, owned func(string) bool, unknown *[]string) {
	t = derefType(t)
	if owned != nil && owned(path) {
		return
	}
	if t == nil || t.Implements(unmarshalerType) || reflect.PointerTo(t).Implements(unmarshalerType) {
		return
	}
	switch t.Kind() {
	case reflect.Struct:
		collectUnknownStructKeys(t, raw, path, owned, unknown)
	case reflect.Map:
		var entries map[string]json.RawMessage
		if json.Unmarshal(raw, &entries) != nil {
			return
		}
		for key, value := range entries {
			collectUnknownKeys(t.Elem(), value, joinKeyPath(path, key), owned, unknown)
		}
	case reflect.Slice, reflect.Array:
		var items []json.RawMessage
		if json.Unmarshal(raw, &items) != nil {
			return
		}
		for i, item := range items {
			collectUnknownKeys(t.Elem(), item, path+"["+strconv.Itoa(i)+"]", owned, unknown)
		}
	default:
	}
}

func collectUnknownStructKeys(t reflect.Type, raw json.RawMessage, path string, owned func(string) bool, unknown *[]string) {
	if collectCanonicalKeys(t, map[string]struct{}{}) {
		return
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return
	}
	for key, value := range fields {
		field, ok := fieldByJSONName(t, key)
		if !ok {
			*unknown = append(*unknown, joinKeyPath(path, key))
			continue
		}
		collectUnknownKeys(field.Type, value, joinKeyPath(path, key), owned, unknown)
	}
}

func joinKeyPath(path, key string) string {
	if path == "" {
		return key
	}
	return path + "." + key
}
