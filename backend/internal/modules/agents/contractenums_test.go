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

// A contract enum a model must spell is served with its values and the
// contract's sentence, and without a value only a response carries. The census
// is every input derived from a Go type, as recorded when it was derived.
func TestEveryContractEnumAnInputTakesIsServedWithItsValuesAndASentence(t *testing.T) {
	gaps, checked := ContractEnumGaps()
	if len(checked) == 0 {
		t.Fatal("no derived input was recorded, so the census checked nothing")
	}
	for _, gap := range gaps {
		t.Error(gap)
	}
}

// The scope kind the contract marks response-only is not offered to a request.
func TestAResponseOnlyScopeKindIsNotServedToARequest(t *testing.T) {
	entry := contractFieldEnums["ReportingScope.kind"]
	if !slices.Contains(entry.ResponseOnly, "managed_teams") || slices.Contains(entry.Values, "managed_teams") {
		t.Fatalf("scope kind values %v, response-only %v", entry.Values, entry.ResponseOnly)
	}
	var served jsonSchema
	if err := json.Unmarshal(reportingReadInput, &served); err != nil {
		t.Fatal(err)
	}
	if slices.Contains(served.Properties["selection"].Properties["scope"].Properties["kind"].Enum, "managed_teams") {
		t.Error("selection.scope.kind offers managed_teams, which a request names by sending no scope")
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

// Deriving an input again, as a rebuilt tool surface does, keeps the census at
// one entry per type instead of growing it.
func TestDerivingAnInputAgainLeavesTheCensusUnchanged(t *testing.T) {
	describedSchema[ReportingRead]("", "")
	_, before := ContractEnumGaps()
	describedSchema[ReportingRead]("", "")
	describedSchema[ReportingRead]("", "")
	if _, after := ContractEnumGaps(); len(after) != len(before) {
		t.Errorf("the census holds %d inputs after deriving one again, was %d", len(after), len(before))
	}
}
