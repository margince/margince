// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

import (
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// The rules exist because the extractor can pick the right FIELD and still
// carry the wrong KIND of value, and both halves matter: a rule that misses
// the real miscategorisation is useless, and one that fires on a good fact
// teaches the reader to ignore the flag.
func TestFactSuspectReasonNamesTheContradictionAndStaysQuietOtherwise(t *testing.T) {
	cases := []struct {
		name  string
		field crmcontracts.CompanyFactField
		value string
		want  string
	}{
		// The one seen in the wild: a contact page offers both fields, so its
		// phone number can land as the company's location.
		{"a phone number filed as a location", crmcontracts.CompanyFactFieldLocation, "+49 30 1234567", "phone_shaped_location"},
		{"a real street address", crmcontracts.CompanyFactFieldLocation, "Ritterstraße 12, 10969 Berlin", ""},
		{"a bare country", crmcontracts.CompanyFactFieldLocation, "Germany", ""},
		// A house number and a postcode together must not reach the digit
		// threshold, or every German address would be flagged.
		{"an address that is mostly digits", crmcontracts.CompanyFactFieldLocation, "Hauptstr. 5, 80331", ""},

		{"a phone that is prose", crmcontracts.CompanyFactFieldPhone, "call us anytime", "not_a_phone"},
		{"a phone in national format", crmcontracts.CompanyFactFieldPhone, "030 / 1234-567", ""},

		{"a founding year that is a register number", crmcontracts.CompanyFactFieldFoundedYear, "HRB 123456", "not_a_year"},
		{"a plausible founding year", crmcontracts.CompanyFactFieldFoundedYear, "1998", ""},
		{"a year outside any company's life", crmcontracts.CompanyFactFieldFoundedYear, "3025", "not_a_year"},

		{"an email with no at sign", crmcontracts.CompanyFactFieldContactEmail, "info at scale.example", "not_an_email"},
		{"an ordinary address", crmcontracts.CompanyFactFieldContactEmail, "info@scale.example", ""},
		// Punctuation alone satisfies "has an @ and has a dot" and is still
		// not an address by any reading.
		{"punctuation that is not an address", crmcontracts.CompanyFactFieldContactEmail, "@.", "not_an_email"},
		{"an address with no local part", crmcontracts.CompanyFactFieldContactEmail, "@scale.example", "not_an_email"},
		{"an address with no dot in the domain", crmcontracts.CompanyFactFieldContactEmail, "info@localhost", "not_an_email"},
		{"an address with two at signs", crmcontracts.CompanyFactFieldContactEmail, "a@b@scale.example", "not_an_email"},
		{"a plus-tagged address", crmcontracts.CompanyFactFieldContactEmail, "sales+eu@scale.example", ""},

		// A register number IS digits, so digits alone cannot separate it from
		// a headcount — what does is what the value leads with.
		{"a register number filed as a headcount", crmcontracts.CompanyFactFieldEmployeeRange, "HRB 123456 B, Amtsgericht Berlin", "not_a_size"},
		// Short enough to pass a length rule, and still a register number.
		{"a short register number", crmcontracts.CompanyFactFieldEmployeeRange, "HRB 123456", "not_a_size"},
		{"a band", crmcontracts.CompanyFactFieldEmployeeRange, "11-50", ""},
		{"an open-ended count", crmcontracts.CompanyFactFieldEmployeeRange, "500+", ""},
		{"a qualified count", crmcontracts.CompanyFactFieldEmployeeRange, "ca. 40", ""},
		// Long enough to fail a length rule, and still an honest headcount.
		{"a spelled-out count phrase", crmcontracts.CompanyFactFieldEmployeeRange, "ca. 5.000 Mitarbeiter weltweit", ""},
		{"a headcount with no digits at all", crmcontracts.CompanyFactFieldEmployeeRange, "several dozen", "not_a_size"},

		// Fields with no rule stay silent rather than guessing.
		{"a service, which has no shape to check", crmcontracts.CompanyFactFieldService, "Managed hosting", ""},
		// Phone punctuation with no digits behind it is not a phone number,
		// and must not drag a location into the flag.
		{"punctuation that is not a number", crmcontracts.CompanyFactFieldLocation, "(.....)", ""},
		{"a short internal extension is not enough", crmcontracts.CompanyFactFieldLocation, "12-345", ""},
		{"an empty value is nothing to judge", crmcontracts.CompanyFactFieldPhone, "   ", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := factSuspectReason(tc.field, tc.value); got != tc.want {
				t.Errorf("factSuspectReason(%s, %q) = %q, want %q", tc.field, tc.value, got, tc.want)
			}
		})
	}
}
