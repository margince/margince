// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"encoding/json"
	"testing"

	"github.com/margince/margince/backend/internal/modules/ai"
)

// The tier enum is derived from the task contract through ai.AllTiers. A schema
// that lists a tier the router does not serve flags a binding that works. One
// that misses a tier passes a binding that will not bind.
func TestTheTierEnumIsDerivedFromTheContractsTiers(t *testing.T) {
	var defs struct {
		AiRouting struct {
			Properties struct {
				Tiers struct {
					//nolint:tagliatelle // "propertyNames" is JSON Schema's own keyword, camelCase by spec
					PropertyNames struct{ Enum []string } `json:"propertyNames"`
				} `json:"tiers"`
			} `json:"properties"`
		} `json:"aiRouting"`
	}
	raw, err := routingDefs()
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &defs); err != nil {
		t.Fatalf("the generated $defs are not parseable: %v", err)
	}
	got := defs.AiRouting.Properties.Tiers.PropertyNames.Enum
	want := ai.AllTiers()
	if len(got) != len(want) {
		t.Fatalf("schema lists %d tiers, the contract declares %d: %v vs %v", len(got), len(want), got, want)
	}
	for i, tier := range want {
		if got[i] != string(tier) {
			t.Errorf("tier %d is %q in the schema and %q in the contract", i, got[i], tier)
		}
	}
	// Not a tolerated zero: the contract declares tiers, so an empty enum means
	// the derivation broke while the schema still looks well-formed.
	if len(got) == 0 {
		t.Fatal("the tier enum is empty — the schema would accept no binding at all")
	}
}
