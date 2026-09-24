// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// Local references in a schema fitted for Anthropic's structured output. The
// decoder resolves a `$ref` to one of the root's own `$defs` or `definitions`,
// and refuses an external reference, a recursive one, and a reference inside
// allOf; this file answers which of those a schema is.

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// localReference refuses a `$ref` the decoder cannot resolve: anything but a
// pointer to one of the root's own definitions. An external URL is refused by
// the vendor, and a pointer into the middle of the schema — "#" itself
// included — is how a schema refers back to itself.
func (f *anthropicSchemaFitter) localReference(value json.RawMessage) error {
	var pointer string
	if json.Unmarshal(value, &pointer) != nil {
		return fmt.Errorf("%w: a reference that is not a string", errSchemaIneligible)
	}
	if _, declared := f.definitions[pointer]; !declared {
		return fmt.Errorf("%w: reference %q is not one of the root's definitions", errSchemaIneligible, pointer)
	}
	return nil
}

// rootDefinitions is the root's local reference targets, keyed by the pointer
// a `$ref` spells to reach each one ("#/$defs/name", "#/definitions/name").
func rootDefinitions(raw json.RawMessage) (map[string]json.RawMessage, error) {
	root, err := decodeSchemaObject(raw)
	if err != nil {
		return nil, err
	}
	definitions := map[string]json.RawMessage{}
	for _, block := range []string{kwDefs, kwDefinitions} {
		value, present := root.lookup(block)
		if !present {
			continue
		}
		byName, err := decodeSchemaObject(value)
		if err != nil {
			return nil, err
		}
		for _, definition := range byName {
			definitions["#/"+block+"/"+definition.key] = definition.value
		}
	}
	return definitions, nil
}

// refuseRecursion refuses definitions whose references lead back to where they
// started, which the decoder does not take. Only the definitions can recurse:
// localReference admits no other target, so a cycle has to run through them.
func refuseRecursion(definitions map[string]json.RawMessage) error {
	edges := make(map[string][]string, len(definitions))
	for pointer, definition := range definitions {
		edges[pointer] = referencesIn(definition)
	}
	// visiting holds the pointers on the current path, done the ones already
	// proved to lead nowhere back.
	visiting, done := map[string]bool{}, map[string]bool{}
	var reaches func(string) bool
	reaches = func(pointer string) bool {
		if visiting[pointer] {
			return true
		}
		if done[pointer] {
			return false
		}
		visiting[pointer] = true
		for _, next := range edges[pointer] {
			if reaches(next) {
				return true
			}
		}
		visiting[pointer], done[pointer] = false, true
		return false
	}
	for pointer := range definitions {
		if reaches(pointer) {
			return fmt.Errorf("%w: definition %q refers back to itself", errSchemaIneligible, pointer)
		}
	}
	return nil
}

// referencesIn is every `$ref` string anywhere inside one JSON value. A value
// that does not decode contributes none; the fit refuses it on its own walk.
func referencesIn(raw json.RawMessage) []string {
	trimmed := bytes.TrimSpace(raw)
	var found []string
	switch {
	case len(trimmed) > 0 && trimmed[0] == '{':
		node, err := decodeSchemaObject(trimmed)
		if err != nil {
			return nil
		}
		for _, member := range node {
			var pointer string
			if member.key == kwRef && json.Unmarshal(member.value, &pointer) == nil {
				found = append(found, pointer)
			}
			found = append(found, referencesIn(member.value)...)
		}
	case len(trimmed) > 0 && trimmed[0] == '[':
		var elements []json.RawMessage
		if json.Unmarshal(trimmed, &elements) != nil {
			return nil
		}
		for _, element := range elements {
			found = append(found, referencesIn(element)...)
		}
	}
	return found
}

// schemaDeclares reports whether raw is a schema object carrying keyword.
func schemaDeclares(raw json.RawMessage, keyword string) bool {
	node, err := decodeSchemaObject(raw)
	if err != nil {
		return false
	}
	_, declared := node.lookup(keyword)
	return declared
}
