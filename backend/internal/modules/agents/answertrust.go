// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"encoding/json"
	"slices"
)

// machineFormats are the string formats whose values no one writes: ids,
// instants and numbers. An address or a link is not one of them, because its
// local part or path is text somebody chose.
var machineFormats = []string{"uuid", "date-time", "date", "int64", "int32", "double"}

// answerNode is the part of a JSON schema that decides whether a value can
// hold somebody's words.
type answerNode struct {
	Type            json.RawMessage            `json:"type"`
	Enum            []json.RawMessage          `json:"enum"`
	Const           json.RawMessage            `json:"const"`
	ExtensibleEnum  bool                       `json:"x-extensible-enum"`
	Format          string                     `json:"format"`
	Properties      map[string]json.RawMessage `json:"properties"`
	AdditionalProps json.RawMessage            `json:"additionalProperties"`
	Items           json.RawMessage            `json:"items"`
	AnyOf           []json.RawMessage          `json:"anyOf"`
	OneOf           []json.RawMessage          `json:"oneOf"`
	AllOf           []json.RawMessage          `json:"allOf"`
}

// CarriesFreeText reports whether an answer of this schema may hold text
// someone wrote. A schema it cannot read, or a member whose shape is left open,
// may hold anything, so it answers true for them.
func CarriesFreeText(schema json.RawMessage) bool {
	var node answerNode
	if len(schema) == 0 || json.Unmarshal(schema, &node) != nil {
		return true
	}
	for _, member := range slices.Concat(node.AnyOf, node.OneOf, node.AllOf) {
		if CarriesFreeText(member) {
			return true
		}
	}
	if (len(node.Enum) > 0 || len(node.Const) > 0) && !node.ExtensibleEnum {
		return false
	}
	types := node.types()
	if len(types) == 0 {
		// Unconstrained, unless its members above already said what it holds.
		return len(node.AnyOf)+len(node.OneOf)+len(node.AllOf) == 0
	}
	return slices.ContainsFunc(types, node.freeAs)
}

// types lists the JSON types the node admits. A node naming none is an object
// when it lists members, and anything at all otherwise.
func (n answerNode) types() []string {
	var one string
	if json.Unmarshal(n.Type, &one) == nil && one != "" {
		return []string{one}
	}
	var many []string
	if json.Unmarshal(n.Type, &many) == nil && len(many) > 0 {
		return many
	}
	if n.Properties != nil {
		return []string{"object"}
	}
	return nil
}

// freeAs reports whether a value of this one type may hold free text, or whether
// the node admits no type the schema names.
func (n answerNode) freeAs(jsonType string) bool {
	switch jsonType {
	case "integer", "number", "boolean", "null":
		return false
	case schemaString:
		return !slices.Contains(machineFormats, n.Format)
	case schemaArray:
		return CarriesFreeText(n.Items)
	case "object":
		return n.objectCarriesFreeText()
	default:
		return true
	}
}

// objectCarriesFreeText reads an object's members. An object listing none is
// opaque; open extra members are read by their own schema, and `true` admits any.
func (n answerNode) objectCarriesFreeText() bool {
	extra := string(n.AdditionalProps)
	if n.Properties == nil && (extra == "" || extra == "false") {
		return true
	}
	if extra == "true" || (extra != "" && extra != "false" && CarriesFreeText(n.AdditionalProps)) {
		return true
	}
	for _, member := range n.Properties {
		if CarriesFreeText(member) {
			return true
		}
	}
	return false
}
