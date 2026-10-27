// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// GET /search spells its record types three times, and each must be the search module's branch table.
//
// These are the `types` filter, each hit's `type` and a page's `types_with_more`.
// Each is an `x-extensible-enum` of core types beside a `pattern` that also
// admits `ext:<unit>:<kind>`, because extension kinds are known at boot.
// A type a list omits is sent against the contract's own schema.
// A type a list invents is a filter that answers 422.
// The Go half is parsed from searchBranches, not restated.

import (
	"os"
	"regexp"
	"slices"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestEverySearchTypeEnumIsTheSearchModulesBranchTable(t *testing.T) {
	t.Parallel()
	searchable := searchableEntitiesFromSource(t, everyBranch)
	slices.Sort(searchable)
	for where, spelling := range searchTypeSpellings(t) {
		enum := slices.Clone(spelling.Enum)
		slices.Sort(enum)
		if !slices.Equal(enum, searchable) {
			t.Errorf("%s's %s enum is %v and the search module answers %v", contractFile, where, enum, searchable)
		}
	}
}

// Each pattern must admit every core type and an extension kind, and nothing
// else. Otherwise a client that validates would refuse a hit the server sent.
func TestEverySearchTypeSpellingAdmitsExtensionKinds(t *testing.T) {
	t.Parallel()
	for where, spelling := range searchTypeSpellings(t) {
		pattern, err := regexp.Compile(spelling.Pattern)
		if err != nil {
			t.Fatalf("%s's %s pattern %q: %v", contractFile, where, spelling.Pattern, err)
		}
		for _, kind := range append(slices.Clone(spelling.Enum), "ext:finance-demo:invoice") {
			if !pattern.MatchString(kind) {
				t.Errorf("%s's %s pattern %q refuses %q", contractFile, where, spelling.Pattern, kind)
			}
		}
		if pattern.MatchString("widget") {
			t.Errorf("%s's %s pattern %q admits a type nothing answers", contractFile, where, spelling.Pattern)
		}
	}
}

// SearchNearMatch.type lists the entities whose branch declares a nearName.
func TestSearchNearMatchTypeEnumIsTheNearNamedBranches(t *testing.T) {
	t.Parallel()
	named := searchableEntitiesFromSource(t, nearNamed)
	enum := propertyEnum(t, "SearchNearMatch", "type", false).Enum
	slices.Sort(named)
	slices.Sort(enum)
	if !slices.Equal(enum, named) {
		t.Errorf("%s's SearchNearMatch.type enum is %v and the near-named branches are %v", contractFile, enum, named)
	}
}

// typeSpelling is one place the contract lists the searchable types: the core
// types it names and the pattern that decides what else it admits.
type typeSpelling struct {
	Enum    []string
	Pattern string
}

// searchTypeSpellings reads every spelling of the searchable types off the
// contract, keyed by where it stands. A spelling that cannot be found fails
// rather than shrinking the census.
func searchTypeSpellings(t *testing.T) map[string]typeSpelling {
	t.Helper()
	spellings := map[string]typeSpelling{
		"SearchResult.type":              propertyEnum(t, "SearchResult", "type", false),
		"SearchResponse.types_with_more": propertyEnum(t, "SearchResponse", "types_with_more", true),
		"/search types parameter":        searchTypesParameterEnum(t),
	}
	for where, spelling := range spellings {
		if len(spelling.Enum) == 0 || spelling.Pattern == "" {
			t.Fatalf("%s declares no %s x-extensible-enum and pattern; the spelling this gate holds has moved", contractFile, where)
		}
	}
	return spellings
}

// spellingOf reads the enum a schema lists, whichever of `enum` and
// `x-extensible-enum` it uses, and the pattern beside it.
func spellingOf(fields contractSchemaFields) typeSpelling {
	return typeSpelling{Enum: slices.Concat(fields.Enum, fields.Extensible), Pattern: fields.Pattern}
}

// propertyEnum reads one component property's spelling, or its items' for an
// array property.
func propertyEnum(t *testing.T, schema, property string, array bool) typeSpelling {
	t.Helper()
	node, ok := contractSchema(t, schema).Properties[property]
	if !ok {
		t.Fatalf("%s's %s has no %s property", contractFile, schema, property)
	}
	var fields struct {
		contractSchemaFields `yaml:",inline"`
		Items                contractSchemaFields `yaml:"items"`
	}
	if err := node.Decode(&fields); err != nil {
		t.Fatalf("reading %s.%s: %v", schema, property, err)
	}
	if array {
		return spellingOf(fields.Items)
	}
	return spellingOf(fields.contractSchemaFields)
}

// searchTypesParameterEnum reads the `types` filter's item spelling off GET /search.
func searchTypesParameterEnum(t *testing.T) typeSpelling {
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
			return spellingOf(param.Schema.Items)
		}
	}
	return typeSpelling{}
}
