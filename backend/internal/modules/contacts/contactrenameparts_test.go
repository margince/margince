// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"slices"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// A rename drops each stored name part the new display name no longer holds, so
// a greeting never reads the old first name. A part it still holds stays.
func TestARenameDropsTheNamePartsTheNewNameNoLongerHolds(t *testing.T) {
	stored := crmcontracts.Contact{FullName: "Stefan Altmann", FirstName: new("Stefan"), LastName: new("Altmann")}
	cases := []struct {
		name      string
		update    UpdateContactInput
		wantClear []string
	}{
		{"another name drops both parts", UpdateContactInput{FullName: new("Maria Neumann")}, []string{"first_name", "last_name"}},
		{"a new surname drops only the last name", UpdateContactInput{FullName: new("Stefan Neumann")}, []string{"last_name"}},
		{"a title or a change of case keeps both", UpdateContactInput{FullName: new("Dr. STEFAN ALTMANN")}, nil},
		{"accents folded keep the part", UpdateContactInput{FullName: new("Stéfan Altmann")}, nil},
		{"the unchanged name is no rename", UpdateContactInput{FullName: new("Stefan Altmann")}, nil},
		{"an update that states a part decides it itself", UpdateContactInput{FullName: new("Maria Neumann"), FirstName: new("Maria")}, nil},
		{"restated stored parts still drop", UpdateContactInput{FullName: new("Maria Neumann"), FirstName: new("Stefan"), LastName: new("Altmann")}, []string{"first_name", "last_name"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := updateAsWritten(stored, tc.update, nil)
			if !slices.Equal(got.Clear, tc.wantClear) {
				t.Fatalf("clears %v, want %v", got.Clear, tc.wantClear)
			}
		})
	}
}

// A hyphenated or two-word surname is held as a run of the new name's words.
func TestARenameKeepsAManyWordPartItStillHolds(t *testing.T) {
	stored := crmcontracts.Contact{FullName: "Zoë Müller-Lüdenscheidt", FirstName: new("Zoë"), LastName: new("Müller-Lüdenscheidt")}
	got := updateAsWritten(stored, UpdateContactInput{FullName: new("Prof. Zoë Müller-Lüdenscheidt")}, nil)
	if len(got.Clear) != 0 {
		t.Fatalf("clears %v, want none", got.Clear)
	}
}
