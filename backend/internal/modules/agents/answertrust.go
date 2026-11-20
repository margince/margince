// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"encoding/json"
	"slices"
)

// formatDateTime is the format of an instant.
const formatDateTime = "date-time"

// machineFormats are the string formats whose values no one writes: ids,
// instants and numbers. An address or a link is not one of them, because its
// local part or path is text somebody chose.
var machineFormats = []string{uuidFormat, formatDateTime, "date", "int64", "int32", "double"}

// CarriesFreeText reports whether an answer of this schema may hold text
// someone wrote. A schema it cannot read, or a member whose shape is left open,
// may hold anything, so it answers true for them.
func CarriesFreeText(schema json.RawMessage) bool {
	var node jsonSchema
	if len(schema) == 0 || json.Unmarshal(schema, &node) != nil {
		return true
	}
	return node.carriesFreeText()
}

// carriesFreeText reads one node. A combinator, a pattern of member names or a
// positional item list is read as open rather than followed.
func (s *jsonSchema) carriesFreeText() bool {
	switch {
	case s == nil:
		return true
	case len(s.AnyOf)+len(s.OneOf)+len(s.AllOf)+len(s.PrefixItems)+len(s.PatternProperties) > 0:
		return true
	case (len(s.Enum) > 0 || len(s.Const) > 0) && !s.ExtensibleEnum:
		return false
	}
	switch s.Type {
	case schemaInteger, schemaNumber, schemaBoolean, schemaNull:
		return false
	case schemaString:
		return !slices.Contains(machineFormats, s.Format)
	case schemaArray:
		return s.Items.carriesFreeText()
	case schemaObject:
		return s.objectCarriesFreeText()
	case "":
		return s.Properties == nil || s.objectCarriesFreeText()
	default:
		return true
	}
}

// objectCarriesFreeText reads an object's members. An object is open unless
// additionalProperties closes it or gives its extra members a shape, as JSON
// Schema reads a missing member.
func (s *jsonSchema) objectCarriesFreeText() bool {
	extra := s.AdditionalProperties
	switch {
	case extra == nil, extra.Closed != nil && *extra.Closed:
		return true
	case extra.Schema != nil && extra.Schema.carriesFreeText():
		return true
	}
	for _, member := range s.Properties {
		if member.carriesFreeText() {
			return true
		}
	}
	return false
}
