// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package magic

import (
	"fmt"
	"os"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestUnplacedBreakdownBoundIsTheContracts(t *testing.T) {
	raw, err := os.ReadFile("../../../api/crm.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var document struct {
		Components struct {
			Schemas map[string]struct {
				Properties map[string]struct {
					MaxItems int `yaml:"maxItems"` //nolint:tagliatelle // OpenAPI schema keywords use camel case.
				} `yaml:"properties"`
			} `yaml:"schemas"`
		} `yaml:"components"`
	}
	if err := yaml.Unmarshal(raw, &document); err != nil {
		t.Fatal(err)
	}
	if bound := document.Components.Schemas["HealthSource"].Properties["unplaced_changes"].MaxItems; bound != maxUnplacedKinds {
		t.Fatalf("the contract bounds unplaced_changes at %d, the source cuts at %d", bound, maxUnplacedKinds)
	}
}

func TestUnplacedBreakdownCutsTheKindsAndKeepsTheWholeTotal(t *testing.T) {
	changes := make([]UnplacedChange, 0, maxUnplacedKinds+10)
	for i := range maxUnplacedKinds + 10 {
		changes = append(changes, UnplacedChange{EntityType: fmt.Sprintf("kind_%02d", i), Action: "update", Count: 2})
	}
	total, breakdown := UnplacedBreakdown(changes)
	if len(breakdown) != maxUnplacedKinds || breakdown[0].EntityType != "kind_00" {
		t.Fatalf("breakdown holds %d kinds starting at %+v, want the first %d", len(breakdown), breakdown[0], maxUnplacedKinds)
	}
	if want := 2 * (maxUnplacedKinds + 10); total != want {
		t.Fatalf("total %d, want every kind counted: %d", total, want)
	}
	if total, breakdown := UnplacedBreakdown(nil); total != 0 || breakdown == nil || len(breakdown) != 0 {
		t.Fatalf("no changes gave total %d and breakdown %v, want 0 and an empty list", total, breakdown)
	}
}
