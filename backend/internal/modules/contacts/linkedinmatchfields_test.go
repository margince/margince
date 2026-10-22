// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestABoxIsStagedTickedOnlyWhenTheContactHasNothingThere(t *testing.T) {
	company, edge := ids.NewV7(), ids.NewV7()
	cases := []struct {
		name               string
		in                 LinkedInMatchFields
		wantTitle, wantJob bool
	}{
		{"empty contact", LinkedInMatchFields{Position: "CTO", EmployerCompanyID: &company}, true, true},
		{"contact has a title", LinkedInMatchFields{Position: "CTO", ContactTitle: "Founder"}, false, false},
		{"contact has an employer", LinkedInMatchFields{EmployerCompanyID: &company, EmployerRelationshipID: &edge}, false, false},
		{"export has no position", LinkedInMatchFields{}, false, false},
	}
	for _, tc := range cases {
		got := stagedOffer(tc.in)
		if got.UseAsTitle != tc.wantTitle || got.UseAsEmployer != tc.wantJob {
			t.Errorf("%s: staged title=%v employer=%v, want %v and %v",
				tc.name, got.UseAsTitle, got.UseAsEmployer, tc.wantTitle, tc.wantJob)
		}
	}
}
