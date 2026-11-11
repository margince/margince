// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package extensionprivacy

import (
	"testing"

	"github.com/margince/margince/backend/pkg/extension"
	"github.com/margince/margince/backend/pkg/extension/jurisdiction"
	"github.com/margince/margince/backend/pkg/extension/privacy"
)

func note(name string) privacy.Table {
	return privacy.Table{
		Name:      name,
		Links:     []privacy.Link{{Column: "contact_id", Record: privacy.Contact}},
		Personal:  []string{"body"},
		Retention: jurisdiction.CommercialCorrespondence,
		AgeColumn: "noted_at",
	}
}

func TestPreflightHoldsADeclarationToItsOwnUnit(t *testing.T) {
	if err := Preflight(extension.Extension{Name: "demo", Privacy: []privacy.Table{note("ext_demo_note")}}); err != nil {
		t.Fatalf("a well-formed declaration was refused: %v", err)
	}
	for name, unit := range map[string]extension.Extension{
		"a unit name with no namespace": {Name: "Not A Name", Privacy: []privacy.Table{note("ext_demo_note")}},
		"another unit's table":          {Name: "demo", Privacy: []privacy.Table{note("ext_other_note")}},
		"one table declared twice":      {Name: "demo", Privacy: []privacy.Table{note("ext_demo_note"), note("ext_demo_note")}},
	} {
		if err := Preflight(unit); err == nil {
			t.Errorf("%s: want a refusal, got none", name)
		}
	}
}
