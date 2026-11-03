// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import "testing"

// A company's number and address are stored as a contact's are parsed, and a
// blank one is no value.
func TestACompanysPhoneAndEmailAreStoredParsed(t *testing.T) {
	t.Parallel()
	text := func(s string) *string { return &s }
	phone, email, err := parseCompanyPhoneAndEmail(text("+49 30 1234567"), text(" Info@Example.COM "))
	if err != nil {
		t.Fatalf("parsing a good number and address: %v", err)
	}
	if phone == nil || *phone != "+49301234567" {
		t.Errorf("phone = %v, want +49301234567", phone)
	}
	if email == nil || *email != "info@example.com" {
		t.Errorf("email = %v, want info@example.com", email)
	}
	for _, blank := range []*string{nil, text(""), text("   ")} {
		if phone, email, err := parseCompanyPhoneAndEmail(blank, blank); err != nil || phone != nil || email != nil {
			t.Errorf("a blank value read as %v, %v, %v; want no value and no error", phone, email, err)
		}
	}
	if _, _, err := parseCompanyPhoneAndEmail(text("not a number"), nil); err == nil {
		t.Error("an unparseable number was accepted")
	}
	if _, _, err := parseCompanyPhoneAndEmail(nil, text("not an address")); err == nil {
		t.Error("an unparseable address was accepted")
	}
}
