// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import "testing"

// An edit records only the sets it supplied. A set that differs because another
// writer moved it is not this edit's change, so its undo never takes it back.
func TestAnEditRecordsOnlyTheSetsItSupplied(t *testing.T) {
	differs := map[string]any{"emails": nil, "phones": nil}
	for _, tc := range []struct {
		name                   string
		update                 UpdateContactInput
		wantEmails, wantPhones bool
	}{
		{"emails only", UpdateContactInput{Emails: []ContactEmailInput{}}, true, false},
		{"phones only", UpdateContactInput{Phones: []ContactPhoneInput{}}, false, true},
		{"both", UpdateContactInput{Emails: []ContactEmailInput{}, Phones: []ContactPhoneInput{}}, true, true},
		{"neither", UpdateContactInput{}, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			emails, phones := setsThisEditRecords(differs, tc.update)
			if emails != tc.wantEmails || phones != tc.wantPhones {
				t.Errorf("records emails %v, phones %v; want %v, %v", emails, phones, tc.wantEmails, tc.wantPhones)
			}
		})
	}
}
