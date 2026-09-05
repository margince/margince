// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// Every worklist category reaches a section of the morning.
//
// BriefSectionOf labels each row with the part of the day it belongs to, and the
// morning feed groups runs of rows by that label. A category the mapping does not
// place gets whatever the default arm returns, which is a real answer for the
// categories that were considered and a wrong one for a category nobody thought
// about — the row is drawn under "move revenue" because that is the fall-through,
// not because anybody decided it belongs there.
//
// THE CORPUS IS DERIVED, NOT LISTED. A gate holding its own copy of the seven
// categories is a second spelling of the enum, and it goes stale the same way the
// mapping would (AGENTS.md, "Reuse before you build" rule 5). So the list comes
// out of crm.yaml, and adding an eighth category fails this gate until somebody
// decides which part of the morning it belongs to.
//
// It cannot fail short: the second test requires the parse to find at least the
// seven the product ships, so a reworded schema that yielded an empty corpus is
// loud rather than green (AGENTS.md rule 8).

import (
	"os"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/margince/margince/backend/internal/compose/attention"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

func TestEveryWorklistCategoryIsPlacedInTheMorning(t *testing.T) {
	t.Parallel()
	for _, category := range worklistCategories(t) {
		item := crmcontracts.WorklistItem{
			Category: crmcontracts.WorklistItemCategory(category),
			Actions:  []crmcontracts.WorklistItemActions{},
		}
		if section := attention.BriefSectionOf(item); section == "" {
			t.Errorf("category %q reaches no brief section.\n"+
				"align: backend/internal/compose/attention/briefsections.go — every category must "+
				"reach a section, because a row drawn with no section is one a grouping client "+
				"cannot place.", category)
		}
	}
}

// The census must not be able to pass by reading nothing.
//
// The floor is not a number typed here. An earlier version said `< 7`, a
// constant that agrees with today's answer and would keep agreeing after an
// eighth category shipped — the guard loosening silently at the moment it was
// most needed, and a second copy of the enum's size besides.
//
// What replaces it is two checks that need no list: the parse must find
// something, and every word it finds must be one the GENERATED validator admits.
// The second is what catches a reworded schema that yields junk. It does NOT
// catch a schema that yields a strict subset — for that the gate would need to
// know how many categories exist, which is the copy being avoided. The census
// above is the guard for that case: a category dropped from crm.yaml is one
// BriefSectionOf still places, and nothing here is judging it any more, but the
// same drop fails the frontend's own generated types loudly.
func TestTheCategoryCorpusIsTheContractsOwn(t *testing.T) {
	t.Parallel()
	found := worklistCategories(t)

	if len(found) == 0 {
		t.Fatal("parsed no worklist categories out of crm.yaml: the schema's shape changed and " +
			"this gate is now judging nothing at all. Fix the parse before trusting the census above.")
	}
	for _, category := range found {
		if !crmcontracts.WorklistItemCategory(category).Valid() {
			t.Errorf("crm.yaml declares category %q, which the generated validator does not know.\n"+
				"align: run `make gen` — the contract and the generated enum have drifted, and this "+
				"gate is judging a corpus the code cannot represent.", category)
		}
	}
}

// worklistCategories reads WorklistItem.category's enum out of the contract.
func worklistCategories(t *testing.T) []string {
	t.Helper()
	var doc struct {
		Components struct {
			Schemas map[string]struct {
				Properties map[string]struct {
					Enum []string `yaml:"enum"`
				} `yaml:"properties"`
			} `yaml:"schemas"`
		} `yaml:"components"`
	}
	raw, err := os.ReadFile("api/crm.yaml")
	if err != nil {
		t.Fatalf("reading the contract: %v", err)
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parsing the contract: %v", err)
	}
	return doc.Components.Schemas["WorklistItem"].Properties["category"].Enum
}
