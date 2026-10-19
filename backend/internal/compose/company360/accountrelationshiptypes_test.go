// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package company360

import (
	"encoding/json"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// A company with no live relationship type is written `[]`: the contract
// declares an array, and a generated client reads null as another shape.
func TestAccountRelationshipTypesAreAnEmptyArrayWhenTheCompanyHasNone(t *testing.T) {
	t.Parallel()
	none := []crmcontracts.CompanyRelationshipTypes{}
	for name, types := range map[string]*[]crmcontracts.CompanyRelationshipTypes{"absent": nil, "empty": &none} {
		wire, err := json.Marshal(accountRelationshipTypes(types))
		if err != nil || string(wire) != "[]" {
			t.Errorf("%s types → %s (%v), want []", name, wire, err)
		}
	}
}

func TestAccountRelationshipTypesKeepTheCompanysTypesInOrder(t *testing.T) {
	t.Parallel()
	held := []crmcontracts.CompanyRelationshipTypes{"customer", "partner"}
	wire, err := json.Marshal(accountRelationshipTypes(&held))
	if err != nil || string(wire) != `["customer","partner"]` {
		t.Errorf("types → %s (%v), want [\"customer\",\"partner\"]", wire, err)
	}
}
