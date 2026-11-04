// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// The `uniqueItems` claims a tool's schema makes about its list arguments, held
// at Registry.Invoke beside the id, range and vocabulary checks. A tool's
// arguments decode into hand-written structs, which carry no contract tag, so
// the schema is the one place the claim can be read from.

import (
	"encoding/json"
	"sort"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// declaredUniqueArgs names the top-level list arguments whose schema declares
// `uniqueItems: true`, read once at registration.
func declaredUniqueArgs(inputSchema json.RawMessage) []string {
	var schema struct {
		Properties map[string]struct {
			UniqueItems bool `json:"uniqueItems"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(inputSchema, &schema); err != nil {
		//craft:ignore panic-in-domain composition-time registration assertion — fires only while cmd wiring runs, never on a request path
		panic("crmagents: input schema declares an unreadable `uniqueItems`: " + err.Error())
	}
	var names []string
	for name, prop := range schema.Properties {
		if prop.UniqueItems {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

// requireDeclaredUniqueness refuses a supplied list that repeats an item. A
// value that is not a list is left to the handler's decode, which names the
// type it wanted.
func (r *Registry) requireDeclaredUniqueness(name string, args json.RawMessage) error {
	r.mu.RLock()
	lists := r.uniqueArgs[name]
	r.mu.RUnlock()
	if len(lists) == 0 {
		return nil
	}
	present, isObject := argsAsObject(args)
	if !isObject {
		return nil
	}
	for _, list := range lists {
		var items []json.RawMessage
		if raw, supplied := present[list]; !supplied || json.Unmarshal(raw, &items) != nil {
			continue
		}
		seen := make(map[string]int, len(items))
		for at, item := range items {
			key := wireItemKey(item)
			if first, found := seen[key]; found {
				refusal := datasource.RepeatedItemRefusal(list, first, at)
				return &BadArgsError{Cause: refusal, Field: list}
			}
			seen[key] = at
		}
	}
	return nil
}

// wireItemKey is an item's JSON value without spelling that changes nothing:
// whitespace, key order, and the letter case of a UUID.
func wireItemKey(item json.RawMessage) string {
	var text string
	if json.Unmarshal(item, &text) == nil {
		if id, err := ids.Parse(text); err == nil {
			return id.String()
		}
		return text
	}
	var value any
	if json.Unmarshal(item, &value) != nil {
		return string(item)
	}
	canonical, err := json.Marshal(value)
	if err != nil {
		return string(item)
	}
	return string(canonical)
}
