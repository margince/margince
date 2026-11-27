// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package importcells

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/migration"
)

func TestAPhoneAndALinkedInURLAreCheckedAndCarriedInTheStoresSpelling(t *testing.T) {
	for _, tc := range []struct {
		object    string
		phone     bool
		raw, want string
		problem   string
	}{
		{migration.ObjectContact, true, "+49 30 1234567", "+49301234567", ""},
		{migration.ObjectLead, true, "0049 30 1234567", "+49301234567", ""},
		{migration.ObjectContact, true, "030 1234567", "", `“030 1234567” needs its country code, like +49 30 1234567`},
		{migration.ObjectContact, true, "+49 30 1", "", "not an E.164 number"},
		{migration.ObjectContact, true, "abc", "", "“abc” is not a phone number."},
		{migration.ObjectLead, true, "call 030 1234567", "", "is not a phone number."},
		{migration.ObjectContact, false, "http://www.linkedin.com/in/ada/?trk=x", "https://www.linkedin.com/in/ada", ""},
		{migration.ObjectContact, false, "https://example.com/in/ada", "", "linkedin.com"},
		{migration.ObjectLead, false, "linkedin.com/in/ada", "https://www.linkedin.com/in/ada", ""},
		{migration.ObjectCompany, false, "https://www.linkedin.com/company/voltaq/about", "https://www.linkedin.com/company/voltaq", ""},
		{migration.ObjectCompany, false, "https://www.linkedin.com/in/ada", "", "linkedin.com/company/"},
	} {
		read := func(text string) (string, migration.Reason) { return LinkedIn(tc.object, text) }
		if tc.phone {
			read = Phone
		}
		got, why := read(tc.raw)
		if tc.problem == "" {
			if why.Code != "" || got != tc.want {
				t.Errorf("%s %q → %q, %+v; want %q", tc.object, tc.raw, got, why, tc.want)
			}
			continue
		}
		if why.Code == "" || !strings.Contains(why.English(), tc.problem) {
			t.Errorf("%s %q → %+v, want a refusal saying %q", tc.object, tc.raw, why, tc.problem)
		}
	}
}
