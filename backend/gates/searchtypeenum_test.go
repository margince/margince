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
	raw, err := os.ReadFile(contractFile)
	if err != nil {
		t.Fatalf("reading %s: %v", contractFile, err)
	}
	type itemsEnum struct {
		Items contractSchemaFields `yaml:"items"`
	}
	var doc struct {
		Paths map[string]struct {
			Get struct {
				Parameters []struct {
					Name   string    `yaml:"name"`
					Schema itemsEnum `yaml:"schema"`
				} `yaml:"parameters"`
			} `yaml:"get"`
		} `yaml:"paths"`
		Components struct {
			Schemas struct {
				SearchResult struct {
					Properties struct {
						Type contractSchemaFields `yaml:"type"`
					} `yaml:"properties"`
				} `yaml:"SearchResult"`
				SearchResponse struct {
					Properties struct {
						TypesWithMore itemsEnum `yaml:"types_with_more"`
					} `yaml:"properties"`
				} `yaml:"SearchResponse"`
			} `yaml:"schemas"`
		} `yaml:"components"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parsing %s: %v", contractFile, err)
	}
	enums := map[string][]string{
		"SearchResult.type":              doc.Components.Schemas.SearchResult.Properties.Type.Enum,
		"SearchResponse.types_with_more": doc.Components.Schemas.SearchResponse.Properties.TypesWithMore.Items.Enum,
	}
	for _, param := range doc.Paths["/search"].Get.Parameters {
		if param.Name == "types" {
			enums["/search types parameter"] = param.Schema.Items.Enum
		}
	}
	for _, where := range []string{"SearchResult.type", "SearchResponse.types_with_more", "/search types parameter"} {
		if len(enums[where]) == 0 {
			t.Fatalf("%s declares no %s enum; the spelling this gate holds has moved", contractFile, where)
		}
	}
	return enums
}
