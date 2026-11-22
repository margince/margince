// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"encoding/json"
	"reflect"
	"slices"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// servedInputs are the report tools' input types whose selection a model fills in.
var servedInputs = map[string]json.RawMessage{
	"read_reporting":   reportingReadInput,
	"draft_report":     reportDraftInput,
	"metric_reference": reportingMetricInput,
}

// A contract enum a model must spell is served with its values and the
// contract's sentence, wherever a body reaches it. This walks the Go type beside
// the served schema, so a field added to the selection is held to it too.
func TestEveryContractEnumAnInputTakesIsServedWithItsValuesAndASentence(t *testing.T) {
	roots := map[string]reflect.Type{
		"read_reporting":   reflect.TypeFor[ReportingRead](),
		"draft_report":     reflect.TypeFor[ReportDraftRequest](),
		"metric_reference": reflect.TypeFor[crmcontracts.ReportMetricReference](),
	}
	for tool, root := range roots {
		var served jsonSchema
		if err := json.Unmarshal(servedInputs[tool], &served); err != nil {
			t.Fatalf("%s: input schema does not parse: %v", tool, err)
		}
		checked := 0
		var visit func(path string, typ reflect.Type, node *jsonSchema)
		visit = func(path string, typ reflect.Type, node *jsonSchema) {
			for typ.Kind() == reflect.Pointer || typ.Kind() == reflect.Slice {
				typ = typ.Elem()
			}
			if typ.Kind() != reflect.Struct || node == nil {
				return
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
				fieldPath := path + "." + name
				if _, open := openValueFields[typ.Name()+"."+name]; !open && isContractEnum(field.Type) {
					checked++
					if len(leaf.Enum) == 0 || member.Description == "" {
						t.Errorf("%s serves %s as a bare string: it has %d values and description %q",
							tool, fieldPath, len(leaf.Enum), member.Description)
					}
				}
				visit(fieldPath, field.Type, leaf)
				return nil
			})
			if err != nil {
				t.Fatalf("%s: %v", path, err)
			}
		}
		visit(tool, root, &served)
		if checked == 0 {
			t.Errorf("%s: the walk met no contract enum, so it checked nothing", tool)
		}
	}
}

// The two selection fields a model guessed wrong carry the contract's
// values. A value the contract does not have is not among them.
func TestSelectionCloseWindowAndValueBasisCarryTheContractsValues(t *testing.T) {
	var served jsonSchema
	if err := json.Unmarshal(reportingReadInput, &served); err != nil {
		t.Fatal(err)
	}
	selection := served.Properties["selection"].Properties
	for field, valid := range map[string]func(string) bool{
		"close_window": func(v string) bool { return crmcontracts.ReportingSelectionCloseWindow(v).Valid() },
		"value_basis":  func(v string) bool { return crmcontracts.ReportingValueBasis(v).Valid() },
	} {
		values := selection[field].Enum
		if len(values) == 0 || selection[field].Description == "" {
			t.Fatalf("selection.%s is served without values or a description", field)
		}
		for _, value := range values {
			if !valid(string(value)) {
				t.Errorf("selection.%s offers %q, which the contract does not accept", field, value)
			}
		}
	}
	if slices.Contains(selection["value_basis"].Enum, "amount") {
		t.Error("selection.value_basis offers amount, a value the contract has no basis for")
	}
}

// An enum the generated table lacks fails the schema instead of serving a bare
// string. The next contract enum in an input cannot slip through.
func TestAContractEnumTheTableLacksFailsTheInputSchema(t *testing.T) {
	type unlisted struct {
		Status crmcontracts.ReportingStatus `json:"status"`
	}
	shape, err := describeType(reflect.TypeFor[unlisted]())
	if err != nil {
		t.Fatal(err)
	}
	if err := noteContractEnums(reflect.TypeFor[unlisted](), shape); err == nil {
		t.Error("a contract enum with no table entry was served without values")
	}
}
