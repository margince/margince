// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"fmt"
	"reflect"
	"strings"
)

// contractFieldEnum is what the contract says about one enum-valued property:
// the values it takes and what they change.
type contractFieldEnum struct {
	Values      []string
	Description string
}

const namedByCatalog = "the catalog mode names the metrics this caller may read"

// openValueFields are contract enum fields a tool input serves without values.
// A catalog names each one, and the values differ per caller. A fixed list
// would be wrong for some callers.
var openValueFields = map[string]string{
	"ReportingSelection.metrics":   namedByCatalog,
	"ReportingSelection.blocks":    "the catalog mode names the chart kinds",
	"ReportingEvidenceRef.metric":  namedByCatalog,
	"ReportMetricReference.metric": namedByCatalog,
	"ReportingCard.kind":           "the catalog mode names the chart kinds",
	"ReportingCard.metric":         namedByCatalog,
}

// noteContractEnums gives every enum-valued field of a tool input its values
// and the contract's sentence, read from the generated contract table.
// A contract enum the table lacks is an error. A new enum in an input cannot be
// served as a bare string.
func noteContractEnums(t reflect.Type, node *jsonSchema) error {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if node == nil {
		return nil
	}
	switch t.Kind() {
	case reflect.Slice, reflect.Array:
		return noteContractEnums(t.Elem(), node.Items)
	case reflect.Map:
		if node.AdditionalProperties != nil {
			return noteContractEnums(t.Elem(), node.AdditionalProperties.Schema)
		}
	case reflect.Struct:
		return eachWireField(t, func(name string, _ bool, field reflect.StructField) error {
			return noteContractField(t, name, field.Type, node.Properties[name])
		})
	}
	return nil
}

func noteContractField(owner reflect.Type, name string, fieldType reflect.Type, node *jsonSchema) error {
	if node == nil {
		return nil
	}
	if !isContractEnum(fieldType) {
		return noteContractEnums(fieldType, node)
	}
	key := owner.Name() + "." + name
	leaf := node
	if leaf.Items != nil {
		leaf = leaf.Items
	}
	if _, open := openValueFields[key]; open || len(leaf.Enum) > 0 {
		return nil
	}
	entry, known := contractFieldEnums[key]
	if !known {
		return fmt.Errorf("%s is a contract enum the generated table does not list; regenerate with make gen", key)
	}
	leaf.Enum = entry.Values
	if node.Description == "" {
		node.Description = entry.Description
	}
	return nil
}

// isContractEnum reports whether a field is typed as one of the contract's
// generated enums: a named string type with a Valid method.
func isContractEnum(t reflect.Type) bool {
	for t.Kind() == reflect.Pointer || t.Kind() == reflect.Slice {
		t = t.Elem()
	}
	if t.Kind() != reflect.String || !strings.Contains(t.PkgPath(), "/internal/contracts") {
		return false
	}
	_, ok := t.MethodByName("Valid")
	return ok
}
