// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"slices"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// A rename drops each stored name part the new display name no longer holds, so
// a greeting never reads the old first name. A part it still holds, a
// hyphenated or two-word surname included, stays.
func TestARenameDropsTheNamePartsTheNewNameNoLongerHolds(t *testing.T) {
	altmann := crmcontracts.Contact{FullName: "Stefan Altmann", FirstName: new("Stefan"), LastName: new("Altmann")}
	zoe := crmcontracts.Contact{FullName: "Zoë Müller-Lüdenscheidt", FirstName: new("Zoë"), LastName: new("Müller-Lüdenscheidt")}
	anna := crmcontracts.Contact{FullName: "Anna van der Meer", FirstName: new("Anna"), LastName: new("van der Meer")}
	cases := []struct {
		name      string
		stored    crmcontracts.Contact
		update    UpdateContactInput
		wantClear []string
	}{
		{"another name drops both parts", altmann, UpdateContactInput{FullName: new("Maria Neumann")}, []string{"first_name", "last_name"}},
		{"a new surname drops only the last name", altmann, UpdateContactInput{FullName: new("Stefan Neumann")}, []string{"last_name"}},
		{"a title or a change of case keeps both", altmann, UpdateContactInput{FullName: new("Dr. STEFAN ALTMANN")}, nil},
		{"accents folded keep the part", altmann, UpdateContactInput{FullName: new("Stéfan Altmann")}, nil},
		{"the unchanged name is no rename", altmann, UpdateContactInput{FullName: new("Stefan Altmann")}, nil},
		{"a stated part decides itself, the other is checked", altmann, UpdateContactInput{FullName: new("Maria Neumann"), FirstName: new("Maria")}, []string{"last_name"}},
		{"a stated part decides itself, a held one stays", altmann, UpdateContactInput{FullName: new("Maria Altmann"), FirstName: new("Maria")}, nil},
		{"a cleared part decides itself", altmann, UpdateContactInput{FullName: new("Maria Neumann"), Clear: []string{"last_name"}}, []string{"last_name", "first_name"}},
		{"restated stored parts still drop", altmann, UpdateContactInput{FullName: new("Maria Neumann"), FirstName: new("Stefan"), LastName: new("Altmann")}, []string{"first_name", "last_name"}},
		{"a hyphenated surname it still holds stays", zoe, UpdateContactInput{FullName: new("Prof. Zoë Müller-Lüdenscheidt")}, nil},
		{"a surname of several words it still holds stays", anna, UpdateContactInput{FullName: new("Dr. Anna van der Meer")}, nil},
		{"a surname of several words it lost drops", anna, UpdateContactInput{FullName: new("Anna van Dijk")}, []string{"last_name"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := updateAsWritten(tc.stored, tc.update, nil)
			if !slices.Equal(got.Clear, tc.wantClear) {
				t.Fatalf("clears %v, want %v", got.Clear, tc.wantClear)
			}
		})
	}
}
