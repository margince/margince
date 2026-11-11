// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build !integration

//gate:kind budget H3

package gates

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

type areaCoupling struct {
	tables, columns, inbound, cascades int
	outbound                           map[string]string
}

func entityModelCoupling(t *testing.T, schema *emSchema, owners map[string]string) map[string]*areaCoupling {
	t.Helper()
	areas := map[string]*areaCoupling{}
	for name, table := range schema.tables {
		area := areaOf(owners[name])
		if areas[area] == nil {
			areas[area] = &areaCoupling{outbound: map[string]string{}}
		}
		areas[area].tables++
		areas[area].columns += len(table.columns)
	}
	for name, table := range schema.tables {
		area := areaOf(owners[name])
		for _, key := range table.foreignKeys {
			parts := catalogForeignKey.FindStringSubmatch(key.def)
			if parts == nil {
				t.Fatalf("cannot measure foreign key %s.%s: %s", name, key.name, key.def)
			}
			parent, exists := schema.tables[parts[2]]
			if !exists {
				t.Fatalf("foreign key %s.%s names missing parent %s", name, key.name, parts[2])
			}
			parentArea := areaOf(owners[parent.name])
			if parentArea == area {
				continue
			}
			areas[parentArea].inbound++
			areas[area].outbound[name+"."+key.name] = key.def
			if strings.Contains(parts[4], "ON DELETE CASCADE") {
				areas[area].cascades++
			}
		}
	}
	return areas
}

func renderEntityModelCoupling(t *testing.T, schema *emSchema, owners map[string]string) string {
	t.Helper()
	areas := entityModelCoupling(t, schema, owners)
	var page strings.Builder
	page.WriteString("# Foreign keys between owning areas\n\n" + generatedBy + "\n\n")
	page.WriteString("[Back to the entity model](README.md).\n\n")
	page.WriteString("Each foreign key crossing an owning-area boundary adds a dependency. Composite keys count once. Internal keys stay within their area and are excluded from the inbound and outbound counts.\n\n")
	page.WriteString("An area with no inbound key is a candidate for a separate module. Its outbound keys need replacement enforcement if moved across the extension boundary. Cascades also need a replacement cleanup path. Calls, events and product scope need a separate review.\n\n")
	page.WriteString("| Area | Tables | Columns | Inbound keys | Outbound keys | Outbound cascades |\n|---|--:|--:|--:|--:|--:|\n")
	for _, area := range sortedKeys(areas) {
		cost := areas[area]
		fmt.Fprintf(&page, "| [%s](%s.md) | %d | %d | %d | %d | %d |\n", area, area, cost.tables, cost.columns, cost.inbound, len(cost.outbound), cost.cascades)
	}
	page.WriteString("\n## Keys from areas with no inbound dependency\n\n")
	for _, area := range sortedKeys(areas) {
		cost := areas[area]
		if cost.inbound != 0 {
			continue
		}
		fmt.Fprintf(&page, "### %s\n\n", area)
		if len(cost.outbound) == 0 {
			page.WriteString("No outbound foreign keys.\n\n")
			continue
		}
		for _, name := range sortedKeys(cost.outbound) {
			fmt.Fprintf(&page, "- `%s`: `%s`\n", name, cost.outbound[name])
		}
		page.WriteString("\n")
	}
	return strings.TrimRight(page.String(), "\n") + "\n"
}

const moduleCouplingPins = "gates/testdata/module-coupling.json"

func couplingGrowth(areas map[string]*areaCoupling, pins map[string]map[string]string) []string {
	var findings []string
	for _, area := range sortedKeys(areas) {
		cost := areas[area]
		before, pinned := pins[area]
		if pinned && cost.inbound > 0 {
			findings = append(findings, fmt.Sprintf("%s now has %d inbound cross-area keys; it was a zero-inbound extraction candidate", area, cost.inbound))
		}
		if !pinned && cost.inbound != 0 {
			continue
		}
		for _, key := range sortedKeys(cost.outbound) {
			if definition, exists := before[key]; !exists || definition != cost.outbound[key] {
				findings = append(findings, fmt.Sprintf("%s extraction dependency added or changed: %s: %s", area, key, cost.outbound[key]))
			}
		}
		for _, key := range sortedKeys(before) {
			if _, exists := cost.outbound[key]; !exists {
				findings = append(findings, fmt.Sprintf("%s extraction dependency removed: %s", area, key))
			}
		}
	}
	for _, area := range sortedKeys(pins) {
		if _, exists := areas[area]; !exists {
			findings = append(findings, fmt.Sprintf("extraction candidate %s disappeared", area))
		}
	}
	return findings
}

func TestEntityModelCouplingDoesNotGrowWithoutReview(t *testing.T) {
	t.Parallel()
	areas := entityModelCoupling(t, parseHeadCatalog(t), tableOwners)
	if *updateEntityModel {
		pins := map[string]map[string]string{}
		for area, cost := range areas {
			if cost.inbound == 0 {
				pins[area] = cost.outbound
			}
		}
		encoded, err := json.MarshalIndent(pins, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(moduleCouplingPins, append(encoded, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	encoded, err := gatekit.ReadFile(moduleCouplingPins)
	if err != nil {
		t.Fatal(err)
	}
	var pins map[string]map[string]string
	if err := json.Unmarshal(encoded, &pins); err != nil {
		t.Fatal(err)
	}
	if len(pins) < 5 {
		t.Fatal("the extraction cost pins cover fewer than five areas")
	}
	for _, finding := range couplingGrowth(areas, pins) {
		t.Errorf("%s. Review the dependency, then acknowledge its cost with %s", finding, regenerate)
	}
}

func TestEntityModelCouplingCountsCompositeKeysAndFindsNewDependencyCosts(t *testing.T) {
	t.Parallel()
	schema := &emSchema{tables: map[string]*emTable{
		"parent": {name: "parent", columns: []emColumn{{name: "id"}}},
		"child":  {name: "child", columns: []emColumn{{name: "a"}, {name: "b"}}, foreignKeys: []emConstraint{{name: "composite", def: "FOREIGN KEY (a, b) REFERENCES parent(id, other) ON DELETE CASCADE"}}},
	}}
	owners := map[string]string{"parent": "internal/modules/one", "child": "internal/modules/two"}
	areas := entityModelCoupling(t, schema, owners)
	cost := areas["two"]
	if cost.tables != 1 || cost.columns != 2 || cost.cascades != 1 || len(cost.outbound) != 1 || areas["one"].inbound != 1 {
		t.Fatalf("wrong coupling: child=%+v parent=%+v", cost, areas["one"])
	}
	if got := couplingGrowth(areas, map[string]map[string]string{"two": {}}); len(got) != 1 || !strings.Contains(got[0], "child.composite") {
		t.Fatalf("new dependency was not named: %v", got)
	}
	if got := couplingGrowth(areas, map[string]map[string]string{"two": {"child.composite": cost.outbound["child.composite"]}}); len(got) != 0 {
		t.Fatalf("acknowledged cost failed: %v", got)
	}
	page := renderEntityModelCoupling(t, schema, owners)
	if !strings.Contains(page, "| [two](two.md) | 1 | 2 | 0 | 1 | 1 |") {
		t.Fatalf("the page omitted the measured cost: %s", page)
	}
}

func TestEntityModelCouplingRejectsReplacedRemovedAndChangedKeys(t *testing.T) {
	t.Parallel()
	before := map[string]map[string]string{"child": {"child.parent_key": "FOREIGN KEY (parent_id) REFERENCES parent(id) ON DELETE SET NULL"}}
	for _, next := range []map[string]string{
		{"child.other_key": "FOREIGN KEY (other_id) REFERENCES other(id) ON DELETE SET NULL"},
		{},
		{"child.parent_key": "FOREIGN KEY (parent_id) REFERENCES parent(id) ON DELETE CASCADE"},
	} {
		if got := couplingGrowth(map[string]*areaCoupling{"child": {outbound: next}}, before); len(got) == 0 {
			t.Fatalf("changed dependency escaped review: %v", next)
		}
	}
}
