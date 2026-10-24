// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"fmt"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

func TestSplitScrapedAddressFindsEachPart(t *testing.T) {
	cases := []struct {
		raw  string
		want crmcontracts.Address
	}{
		{
			"Example Street 1, Tower B, 12345 Example City, Germany",
			crmcontracts.Address{Line1: new("Example Street 1"), Line2: new("Tower B"), PostalCode: new("12345"), City: new("Example City"), Country: new("DE")},
		},
		{
			"Wittekindstr. 1a, 45131 Essen",
			crmcontracts.Address{Line1: new("Wittekindstr. 1a"), PostalCode: new("45131"), City: new("Essen")},
		},
		{
			"Hafenstrasse 12, 20457 Hamburg, Deutschland",
			crmcontracts.Address{Line1: new("Hafenstrasse 12"), PostalCode: new("20457"), City: new("Hamburg"), Country: new("DE")},
		},
		{
			"Stephansplatz 1, 1010 Wien, Österreich",
			crmcontracts.Address{Line1: new("Stephansplatz 1"), PostalCode: new("1010"), City: new("Wien"), Country: new("AT")},
		},
		{
			"Bahnhofstrasse 10; CH-8001 Zürich; Schweiz",
			crmcontracts.Address{Line1: new("Bahnhofstrasse 10"), PostalCode: new("8001"), City: new("Zürich"), Country: new("CH")},
		},
		{
			"Example Street 1 Tower B 12345 Example City Germany",
			crmcontracts.Address{Line1: new("Example Street 1 Tower B"), PostalCode: new("12345"), City: new("Example City"), Country: new("DE")},
		},
		{
			"Example Street 1\nc/o Example Office\n12345 Example City\nDE",
			crmcontracts.Address{Line1: new("Example Street 1"), Line2: new("c/o Example Office"), PostalCode: new("12345"), City: new("Example City"), Country: new("DE")},
		},
		{
			"Postfach 10 01 01, D-10115 Berlin",
			crmcontracts.Address{Line1: new("Postfach 10 01 01"), PostalCode: new("10115"), City: new("Berlin")},
		},
		{
			"1600 Example Parkway, Suite 200, Mountain View, CA 94043, USA",
			crmcontracts.Address{Line1: new("1600 Example Parkway"), Line2: new("Suite 200"), City: new("Mountain View"), Region: new("CA"), PostalCode: new("94043"), Country: new("US")},
		},
		{
			"PO Box 123, Springfield, IL 62701-1234",
			crmcontracts.Address{Line1: new("PO Box 123"), City: new("Springfield"), Region: new("IL"), PostalCode: new("62701-1234")},
		},
		{
			"Số 1 Lê Duẩn, Phường Bến Nghé, Quận 1, Thành phố Hồ Chí Minh 700000, Việt Nam",
			crmcontracts.Address{Line1: new("Số 1 Lê Duẩn"), Line2: new("Phường Bến Nghé, Quận 1"), City: new("Thành phố Hồ Chí Minh"), PostalCode: new("700000"), Country: new("VN")},
		},
	}
	for _, tc := range cases {
		got, ok := SplitAddressText(tc.raw)
		if !ok {
			t.Errorf("SplitAddressText(%q) refused an address with a street, postal code and city", tc.raw)
			continue
		}
		if got, want := addressPartsOf(got), addressPartsOf(tc.want); got != want {
			t.Errorf("SplitAddressText(%q)\n got %q\nwant %q", tc.raw, got, want)
		}
	}
}

// Each of these would need a guess to split, so the whole text stays in the
// profile field instead.
func TestSplitScrapedAddressRefusesWhatItWouldHaveToGuess(t *testing.T) {
	for _, raw := range []string{
		"",
		"Example City",
		"Website Road 2",
		"1600 Example Parkway, Suite 200, CA 94043, USA",
		"Germany",
		"Street 1, 12345 City, Atlantis",
		"P.O. Box 1234",
		"Postfach 1234, Example City",
		"Example Street 1, Quận 1, Hà Nội, Việt Nam",
		"Herengracht 1, 1012 AB Amsterdam",
		"10 Example Road, London SW1A 1AA, UK",
		"Example Road 1600 Building 12345 Example City",
		"Example Street 1 12345",
	} {
		if got, ok := SplitAddressText(raw); ok || addressPartsOf(got) != addressPartsOf(crmcontracts.Address{}) {
			t.Errorf("SplitAddressText(%q) = %s, true; want a refusal with nothing filled", raw, fmt.Sprint(addressPartsOf(got)))
		}
	}
}

// addressPartsOf spells each column, with "<nil>" for one left unset, so an
// empty string and a missing part cannot pass for each other.
func addressPartsOf(a crmcontracts.Address) [6]string {
	var out [6]string
	for i, part := range []*string{a.Line1, a.Line2, a.Region, a.PostalCode, a.City, a.Country} {
		out[i] = "<nil>"
		if part != nil {
			out[i] = *part
		}
	}
	return out
}
