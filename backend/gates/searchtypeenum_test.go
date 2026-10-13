// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// GET /search spells its record types three times, and each must be the search module's branch table.
//
// The `types` filter, each hit's `type` and a grouped page's `types_with_more`
// are three hand-typed enums. A type a branch answers and an enum omits is
// sent against the contract's own schema; a type an enum invents is a filter
// that answers 422. The Go half is parsed from searchBranches, as the context
// anchor gate parses it, rather than restated.

import (
	"os"
	"slices"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestEverySearchTypeEnumIsTheSearchModulesBranchTable(t *testing.T) {
	t.Parallel()
	searchable := searchableEntitiesFromSource(t, everyBranch)
	slices.Sort(searchable)
	for where, enum := range searchTypeEnums(t) {
		slices.Sort(enum)
		if !slices.Equal(enum, searchable) {
			t.Errorf("%s's %s enum is %v and the search module answers %v", contractFile, where, enum, searchable)
		}
	}
}

// searchTypeEnums reads every spelling of the searchable types off the
// contract, keyed by where it stands. A spelling that cannot be found fails
// rather than shrinking the census.
func searchTypeEnums(t *testing.T) map[string][]string {
	t.Helper()
	enums := map[string][]string{
		"SearchResult.type":              propertyEnum(t, "SearchResult", "type", false),
		"SearchResponse.types_with_more": propertyEnum(t, "SearchResponse", "types_with_more", true),
		"/search types parameter":        searchTypesParameterEnum(t),
	}
	for where, enum := range enums {
		if len(enum) == 0 {
			t.Fatalf("%s declares no %s enum; the spelling this gate holds has moved", contractFile, where)
		}
	}
	return enums
}

// propertyEnum reads one component property's enum, or its items' enum for an
// array property.
func propertyEnum(t *testing.T, schema, property string, array bool) []string {
	t.Helper()
	node, ok := contractSchema(t, schema).Properties[property]
	if !ok {
		t.Fatalf("%s's %s has no %s property", contractFile, schema, property)
	}
	var fields struct {
		Enum  []string             `yaml:"enum"`
		Items contractSchemaFields `yaml:"items"`
	}
	if err := node.Decode(&fields); err != nil {
		t.Fatalf("reading %s.%s: %v", schema, property, err)
	}
	if array {
		return fields.Items.Enum
	}
	return fields.Enum
}

// searchTypesParameterEnum reads the `types` filter's item enum off GET /search.
func searchTypesParameterEnum(t *testing.T) []string {
	t.Helper()
	raw, err := os.ReadFile(contractFile)
	if err != nil {
		t.Fatalf("reading %s: %v", contractFile, err)
	}
	var doc struct {
		Paths map[string]struct {
			Get struct {
				Parameters []struct {
					Name   string `yaml:"name"`
					In     string `yaml:"in"`
					Schema struct {
						Items contractSchemaFields `yaml:"items"`
					} `yaml:"schema"`
				} `yaml:"parameters"`
			} `yaml:"get"`
		} `yaml:"paths"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parsing %s: %v", contractFile, err)
	}
	for _, param := range doc.Paths["/search"].Get.Parameters {
		if param.Name == "types" && param.In == "query" {
			return param.Schema.Items.Enum
		}
	}
	return nil
}
