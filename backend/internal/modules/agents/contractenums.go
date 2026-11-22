// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"encoding/json"
	"fmt"
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

// reflectedInputs are the inputs derived from a Go type, with what each was
// served as. ContractEnumGaps reads them, so a census covers every such input
// rather than a list kept beside them.
var reflectedInputs = struct {
	mu   sync.Mutex
	list []reflectedInput
}{}

type reflectedInput struct {
	typ    reflect.Type
	schema json.RawMessage
}

func recordReflectedInput(typ reflect.Type, schema json.RawMessage) {
	reflectedInputs.mu.Lock()
	defer reflectedInputs.mu.Unlock()
	reflectedInputs.list = append(reflectedInputs.list, reflectedInput{typ: typ, schema: schema})
}

// ContractEnumGaps names every contract enum field of an input derived from a Go
// type that is served with a defect. The defects are no values, no sentence, or
// a value the contract marks response-only. It also returns the types it checked.
// A gate can tell a full census from an empty one.
func ContractEnumGaps() (gaps, checked []string) {
	reflectedInputs.mu.Lock()
	inputs := slices.Clone(reflectedInputs.list)
	reflectedInputs.mu.Unlock()
	for _, input := range inputs {
		var served jsonSchema
		if err := json.Unmarshal(input.schema, &served); err != nil {
			gaps = append(gaps, fmt.Sprintf("%s: the served schema does not parse: %v", input.typ, err))
			continue
		}
		checked = append(checked, input.typ.String())
		gaps = append(gaps, enumGapsIn(input.typ.String(), input.typ, &served)...)
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
