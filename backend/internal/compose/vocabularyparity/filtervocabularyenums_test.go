// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package vocabularyparity

// Filter types and operators must match the predicate engine in both directions.
// Compose may read the contract and platform code in the same test.

import (
	"testing"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

// vocabularyFieldEnum reads one inline enum off the FilterVocabularyField
// schema. property names the property; itemsLevel says whether the enum sits on
// the property itself (`type`) or on its array items (`operators`).
func vocabularyFieldEnum(t *testing.T, property string, itemsLevel bool) map[string]bool {
	t.Helper()
	doc, err := openapi3.NewLoader().LoadFromFile("../../../api/crm.yaml")
	if err != nil {
		t.Fatalf("load contract: %v", err)
	}
	schema := doc.Components.Schemas["FilterVocabularyField"]
	if schema == nil || schema.Value == nil {
		t.Fatal("FilterVocabularyField is absent from the contract — the vocabulary read has no response shape")
	}
	prop := schema.Value.Properties[property]
	if prop == nil || prop.Value == nil {
		t.Fatalf("FilterVocabularyField has no %q property", property)
	}
	carrier := prop.Value
	if itemsLevel {
		if carrier.Items == nil || carrier.Items.Value == nil {
			t.Fatalf("FilterVocabularyField.%s declares no items, so it carries no enum", property)
		}
		carrier = carrier.Items.Value
	}
	if len(carrier.Enum) == 0 {
		t.Fatalf("FilterVocabularyField.%s is not a closed enum, so a client cannot know what to expect", property)
	}
	values := make(map[string]bool, len(carrier.Enum))
	for _, raw := range carrier.Enum {
		value, ok := raw.(string)
		if !ok {
			t.Fatalf("FilterVocabularyField.%s has a non-string enum member %#v", property, raw)
		}
		values[value] = true
	}
	return values
}

func TestTheVocabularysOperatorEnumIsExactlyWhatTheEngineAdmits(t *testing.T) {
	engine := map[string]bool{}
	for _, fieldType := range everyFilterableFieldType() {
		// Base-table fields: the contract enum has to carry every operator ANY
		// field can report, and a linked field only ever reports a subset of
		// what its own type admits.
		for _, op := range storekit.OperatorsFor(storekit.Field{Type: fieldType}) {
			engine[op] = true
		}
	}
	compareVocabularySets(t, "operator", engine, vocabularyFieldEnum(t, "operators", true),
		"the engine admits it and the contract cannot carry it, so the vocabulary would report a value no client can read",
		"the contract advertises it and no field type admits it, so no field can ever report it")
}

func TestTheVocabularysTypeEnumIsExactlyWhatIsFilterable(t *testing.T) {
	engine := map[string]bool{}
	for _, fieldType := range everyFilterableFieldType() {
		engine[string(fieldType)] = true
	}
	compareVocabularySets(t, "field type", engine, vocabularyFieldEnum(t, "type", false),
		"it is filterable and the contract cannot spell it",
		"the contract advertises it and nothing filterable carries it")
}

func TestTheVocabularysReferenceEnumIsExactlyWhatTheEngineDeclares(t *testing.T) {
	engine := map[string]bool{}
	for _, target := range storekit.ReferenceTargets() {
		engine[string(target)] = true
	}
	compareVocabularySets(t, "reference target", engine,
		vocabularyFieldEnum(t, "references", false),
		"the engine points id fields at it and the contract cannot spell it, so the vocabulary would report a value no client can read",
		"the contract advertises it and no id field points at it, so no field can ever report it")
}

// everyFilterableFieldType is the custom-field types, derived from
// fieldcatalog.Types(), plus the ones only a CORE field carries.
//
// The core-only half is listed, because there is nowhere to derive it from: a
// custom field cannot be an id or a domain — the catalogue offers neither — so
// no catalogue read would ever report them. Listing them is what makes this
// helper's set the same set the contract enum must carry, and adding a third
// core-only type here is the deliberate act of widening a public enum.
var coreOnlyFieldTypes = []storekit.FieldType{
	// A reference to another record.
	storekit.FieldID,
	// A host column, whose operand is folded to the host the column stores.
	storekit.FieldDomain,
	storekit.FieldDistance,
}

func everyFilterableFieldType() []storekit.FieldType {
	types := append([]storekit.FieldType{}, coreOnlyFieldTypes...)
	for _, declared := range fieldcatalog.Types() {
		types = append(types, storekit.FieldType(declared))
	}
	return types
}

func compareVocabularySets(t *testing.T, subject string, engine, contract map[string]bool, missingWhy, extraWhy string) {
	t.Helper()
	for value := range engine {
		if !contract[value] {
			t.Errorf("%s %q: %s", subject, value, missingWhy)
		}
	}
	for value := range contract {
		if !engine[value] {
			t.Errorf("%s %q: %s", subject, value, extraWhy)
		}
	}
}
