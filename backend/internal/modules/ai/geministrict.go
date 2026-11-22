// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// Gemini's constrained decoder leaves out the optional properties of an object.
// An agent's log_activity call carried `kind` and `subject`, never the `body`
// its store requires. So on the Gemini wire every optional property that does
// not admit null is sent as required and nullable. The nulls the decoder then
// writes are taken back out of the reply. A property that admits null
// stays optional: its absence and its null mean different things. The padding
// is done only where the walk that removes it reads. Other wires are unchanged.

import (
	"bytes"
	"encoding/json"
	"slices"
)

// The JSON Schema keywords this file reads beside anthropicschema.go's.
const (
	kwRequired = "required"
	kwOneOf    = "oneOf"
	kwEnum     = "enum"
	kwNullType = "null"
)

// GeminiStrictSchema is raw with every optional property that does not admit
// null made required and nullable. A schema it cannot read is answered with an
// error, and the caller sends it as it was. Exported with GeminiDropPaddedNulls
// so a test over every tool's schema can hold the two to one another.
func GeminiStrictSchema(raw json.RawMessage) (json.RawMessage, error) {
	node, err := decodeSchemaObject(raw)
	if err != nil {
		return nil, err
	}
	out := make(schemaObject, 0, len(node)+1)
	var padded []string
	for _, member := range node {
		value, err := geminiStrictKeyword(node, member.key, member.value)
		if err != nil {
			return nil, err
		}
		if member.key == kwProperties {
			padded = paddedProperties(member.value, requiredOf(node))
		}
		if member.key != kwRequired {
			out = append(out, schemaMember{key: member.key, value: value})
		}
	}
	if _, declared := node.lookup(kwProperties); declared {
		required, err := json.Marshal(append(slices.Clone(requiredOf(node)), padded...))
		if err != nil {
			return nil, err
		}
		out = out.with(kwRequired, required)
	}
	return out.encode()
}

// geminiStrictKeyword rewrites one keyword's value, recursing into every
// keyword that nests a schema.
func geminiStrictKeyword(node schemaObject, key string, value json.RawMessage) (json.RawMessage, error) {
	switch key {
	case kwProperties:
		return geminiStrictProperties(value, requiredOf(node))
	case kwItems:
		if !isJSONObject(value) {
			return value, nil
		}
		return GeminiStrictSchema(value)
	case kwAnyOf, kwOneOf:
		return geminiStrictEach(value)
	default:
		return value, nil
	}
}

// geminiStrictProperties makes each property strict, and an optional one that
// does not admit null nullable as well.
func geminiStrictProperties(raw json.RawMessage, required []string) (json.RawMessage, error) {
	props, err := decodeSchemaObject(raw)
	if err != nil {
		return nil, err
	}
	out := make(schemaObject, 0, len(props))
	for _, prop := range props {
		strict, err := GeminiStrictSchema(prop.value)
		if err != nil {
			return nil, err
		}
		if isPadded(prop, required) {
			strict = json.RawMessage(`{"anyOf":[` + string(strict) + `,{"type":"null"}]}`)
		}
		out = append(out, schemaMember{key: prop.key, value: strict})
	}
	return out.encode()
}

func geminiStrictEach(raw json.RawMessage) (json.RawMessage, error) {
	var branches []json.RawMessage
	if err := json.Unmarshal(raw, &branches); err != nil {
		return nil, err
	}
	for i, branch := range branches {
		strict, err := GeminiStrictSchema(branch)
		if err != nil {
			return nil, err
		}
		branches[i] = strict
	}
	return json.Marshal(branches)
}

// GeminiDropPaddedNulls takes out of reply each null written for a property
// GeminiStrictSchema made nullable. It reads the caller's schema, so a property
// that admits null there keeps its null. A reply that is not JSON is returned as
// it came, for the caller's own parser to refuse.
func GeminiDropPaddedNulls(callerSchema json.RawMessage, reply string) string {
	node, err := decodeSchemaObject(callerSchema)
	if err != nil {
		return reply
	}
	trimmed := bytes.TrimSpace([]byte(reply))
	if !json.Valid(trimmed) {
		return reply
	}
	return string(dropPaddedNulls(node, trimmed))
}

// dropPaddedNulls walks value as node describes it. An object under anyOf or
// oneOf is read by the branch it matches. That is how a step's tool call is read
// by its own tool's arguments.
func dropPaddedNulls(node schemaObject, value json.RawMessage) json.RawMessage {
	if branch, found := matchingBranch(node, value); found {
		return dropPaddedNulls(branch, value)
	}
	if items, ok := node.lookup(kwItems); ok && isJSONArray(value) {
		return dropInItems(items, value)
	}
	props, ok := node.lookup(kwProperties)
	if !ok || !isJSONObject(value) {
		return value
	}
	declared, err := decodeSchemaObject(props)
	if err != nil {
		return value
	}
	members, err := decodeSchemaObject(value)
	if err != nil {
		return value
	}
	required := requiredOf(node)
	kept := make(schemaObject, 0, len(members))
	for _, member := range members {
		prop, known := declared.lookup(member.key)
		switch {
		case !known:
			kept = append(kept, member)
		case isNull(member.value) && isPadded(schemaMember{key: member.key, value: prop}, required):
			continue
		default:
			if sub, err := decodeSchemaObject(prop); err == nil {
				member.value = dropPaddedNulls(sub, member.value)
			}
			kept = append(kept, member)
		}
	}
	encoded, err := kept.encode()
	if err != nil {
		return value
	}
	return encoded
}

func dropInItems(items, value json.RawMessage) json.RawMessage {
	sub, err := decodeSchemaObject(items)
	if err != nil {
		return value
	}
	var elements []json.RawMessage
	if err := json.Unmarshal(value, &elements); err != nil {
		return value
	}
	for i, element := range elements {
		elements[i] = dropPaddedNulls(sub, element)
	}
	encoded, err := json.Marshal(elements)
	if err != nil {
		return value
	}
	return encoded
}

// matchingBranch is the first anyOf or oneOf branch value fits. Every key it
// requires is present, no key it closes out is, and each enum holds one of its
// values. The strict wire wrote every key, so a branch is told by its enums.
func matchingBranch(node schemaObject, value json.RawMessage) (schemaObject, bool) {
	for _, key := range []string{kwAnyOf, kwOneOf} {
		raw, ok := node.lookup(key)
		if !ok {
			continue
		}
		var branches []json.RawMessage
		if json.Unmarshal(raw, &branches) != nil {
			return nil, false
		}
		for _, branch := range branches {
			if obj, err := decodeSchemaObject(branch); err == nil && fits(obj, value) {
				return obj, true
			}
		}
	}
	return nil, false
}

func fits(branch schemaObject, value json.RawMessage) bool {
	members, err := decodeSchemaObject(value)
	if err != nil {
		return false
	}
	declared, err := decodeSchemaObject(branch.get(kwProperties))
	if err != nil {
		return false
	}
	for _, key := range requiredOf(branch) {
		if _, present := members.lookup(key); !present {
			return false
		}
	}
	closed := string(branch.get(kwAdditionalProperties)) == "false"
	for _, member := range members {
		prop, known := declared.lookup(member.key)
		if !known {
			if closed {
				return false
			}
			continue
		}
		if !inEnum(prop, member.value) && !(isNull(member.value) && isPadded(schemaMember{key: member.key, value: prop}, requiredOf(branch))) {
			return false
		}
	}
	return true
}

// inEnum says value is one of prop's enum values, or prop declares none.
func inEnum(prop, value json.RawMessage) bool {
	node, err := decodeSchemaObject(prop)
	if err != nil {
		return true
	}
	raw, declared := node.lookup(kwEnum)
	if !declared {
		return true
	}
	var values []json.RawMessage
	if json.Unmarshal(raw, &values) != nil {
		return true
	}
	return slices.ContainsFunc(values, func(v json.RawMessage) bool {
		return bytes.Equal(bytes.TrimSpace(v), bytes.TrimSpace(value))
	})
}

// admitsNull says a property schema already lets its value be null.
func admitsNull(prop json.RawMessage) bool {
	node, err := decodeSchemaObject(prop)
	if err != nil {
		return false
	}
	var types []string
	raw := node.get(kwType)
	if json.Unmarshal(raw, &types) != nil {
		var one string
		if json.Unmarshal(raw, &one) == nil {
			types = []string{one}
		}
	}
	if slices.Contains(types, kwNullType) || string(node.get("nullable")) == "true" {
		return true
	}
	for _, key := range []string{kwAnyOf, kwOneOf} {
		var branches []json.RawMessage
		if json.Unmarshal(node.get(key), &branches) == nil && slices.ContainsFunc(branches, admitsNull) {
			return true
		}
	}
	return false
}

// paddedProperties are the properties of a properties object that the strict
// wire makes required: optional ones that do not admit null, in their order.
func paddedProperties(raw json.RawMessage, required []string) []string {
	props, err := decodeSchemaObject(raw)
	if err != nil {
		return nil
	}
	var padded []string
	for _, prop := range props {
		if isPadded(prop, required) {
			padded = append(padded, prop.key)
		}
	}
	return padded
}

// isPadded says the strict wire sends prop required and nullable, so a null
// the decoder writes for it is padding and not the model's answer.
func isPadded(prop schemaMember, required []string) bool {
	return !slices.Contains(required, prop.key) && !admitsNull(prop.value)
}

func isNull(value json.RawMessage) bool {
	return string(bytes.TrimSpace(value)) == kwNullType
}

func requiredOf(node schemaObject) []string {
	var required []string
	if json.Unmarshal(node.get(kwRequired), &required) != nil {
		return nil
	}
	return required
}

func isJSONObject(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	return len(trimmed) > 0 && trimmed[0] == '{'
}

func isJSONArray(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	return len(trimmed) > 0 && trimmed[0] == '['
}
