// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"encoding/json"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"
	"sync"
)

// contractFieldEnum is what the contract says about one enum-valued property:
// the values it takes and what they change.
type contractFieldEnum struct {
	Values []string
	// ResponseOnly are members the server answers with and a request never
	// offers; the contract marks them.
	ResponseOnly []string
	Description  string
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

// reflectedInputs holds one entry per Go type an input was derived from, so
// deriving the same type again changes nothing. ContractEnumGaps reads it.
var reflectedInputs = struct {
	mu     sync.Mutex
	byType map[reflect.Type]json.RawMessage
}{byType: map[reflect.Type]json.RawMessage{}}

func recordReflectedInput(typ reflect.Type, schema json.RawMessage) {
	reflectedInputs.mu.Lock()
	defer reflectedInputs.mu.Unlock()
	if _, seen := reflectedInputs.byType[typ]; !seen {
		reflectedInputs.byType[typ] = schema
	}
}

// ContractEnumGaps names every contract enum field of an input derived from a Go
// type that is served with a defect. The defects are no values, no sentence, or
// a value the contract marks response-only. It also returns the types it checked.
// A gate can tell a full census from an empty one.
func ContractEnumGaps() (gaps, checked []string) {
	reflectedInputs.mu.Lock()
	inputs := maps.Clone(reflectedInputs.byType)
	reflectedInputs.mu.Unlock()
	for _, typ := range slices.SortedFunc(maps.Keys(inputs), func(a, b reflect.Type) int { return strings.Compare(a.String(), b.String()) }) {
		var served jsonSchema
		if err := json.Unmarshal(inputs[typ], &served); err != nil {
			gaps = append(gaps, fmt.Sprintf("%s: the served schema does not parse: %v", typ, err))
			continue
		}
		checked = append(checked, typ.String())
		gaps = append(gaps, enumGapsIn(typ.String(), typ, &served)...)
	}
	return gaps, checked
}

func enumGapsIn(path string, typ reflect.Type, node *jsonSchema) (gaps []string) {
	for typ.Kind() == reflect.Pointer || typ.Kind() == reflect.Slice {
		typ = typ.Elem()
	}
	if typ.Kind() != reflect.Struct || node == nil {
		return nil
	}
	err := eachWireField(typ, func(name string, _ bool, field reflect.StructField) error {
		member := node.Properties[name]
		if member == nil {
			return nil
		}
		leaf := member
		if leaf.Items != nil {
			leaf = leaf.Items
		}
		key, fieldPath := typ.Name()+"."+name, path+"."+name
		if _, open := openValueFields[key]; !open && isContractEnum(field.Type) {
			if len(leaf.Enum) == 0 || member.Description == "" {
				gaps = append(gaps, fmt.Sprintf("%s has %d values and description %q", fieldPath, len(leaf.Enum), member.Description))
			}
			for _, hidden := range contractFieldEnums[key].ResponseOnly {
				if slices.Contains(leaf.Enum, hidden) {
					gaps = append(gaps, fmt.Sprintf("%s offers %s, which the contract says only a response carries", fieldPath, hidden))
				}
			}
		}
		gaps = append(gaps, enumGapsIn(fieldPath, field.Type, leaf)...)
		return nil
	})
	if err != nil {
		gaps = append(gaps, fmt.Sprintf("%s: %v", path, err))
	}
	return gaps
}
