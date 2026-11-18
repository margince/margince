// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package salutation

import "testing"

const aliceSignature = "Best regards\nAlice Example\nFounder\n+49 30 5550100\n"

func TestSignerOfTakesTheAddressAloneOnlyWhenNoOtherSignerShows(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name, block, fullName, email string
		want                         Signer
	}{
		{"their name", "Viele Grüße\nJudith Andresen\nGeschäftsführerin", "Judith Andresen", "judith@example.test", SignedByName},
		{"their name without its accents", "Best\nNguyen Van An\nCEO", "Nguyễn Văn An", "an@example.vn", SignedByName},
		{"their address and nothing else", "Founder\n+49 30 1\nbob@example.org", "Bob Contact", "bob@example.org", SignedByAddress},
		{"their address beside another signer's", aliceSignature + "alice@example.com\nbob@example.org", "Bob", "bob@example.org", SignedByNobody},
		{"their name only inside their own address", aliceSignature + "bob@example.org", "Bob", "bob@example.org", SignedByAddress},
		{"their name beside a colleague's address", "Best\nBob Contact\nbob@example.org\nteam@example.org", "Bob Contact", "bob@example.org", SignedByName},
		{"a colleague sharing the surname", "Best\nAnn Jones\nCEO", "Ann Smith", "ann@example.org", SignedByNobody},
		{"a longer name containing theirs", "Viele Grüße\nJoanne Brown\nCEO", "Ann Smith", "ann@example.org", SignedByNobody},
		{"a different contact entirely", "Best\nMarcus Greven\nPartner Manager", "Ann Smith", "ann@example.org", SignedByNobody},
		{"no signature", "", "Ann Smith", "ann@example.org", SignedByNobody},
		{"their name behind an initial", "Best\nJohn Smith\nCTO", "J. Smith", "", SignedByName},
		{"their name behind a nickname", "Best\nRobert Smith\nCTO", "Bob Smith", "", SignedByName},
		{"a colleague's line sharing the surname", "Best\nAnn Jones\nBonn office", "Bob Jones", "", SignedByNobody},
		{"a reply header naming them", "Bob <bob@example.org> wrote:\nBest\nAlice Example", "Bob", "bob@example.org", SignedByNobody},
		{"a short name of their own", "Regards\nLi Bo\nCEO", "Li Bo", "", SignedByName},
		{"a longer name containing a short one", "Regards\nLiam Bosch\nCEO", "Li Bo", "", SignedByNobody},
	} {
		if got := SignerOf(c.block, c.fullName, c.email); got != c.want {
			t.Errorf("%s: SignerOf = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestNameIsContactsKnowsTheSameContactAndRefusesAnother(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name, given, family, fullName, email string
		want                                 bool
	}{
		{"the header name in printed order", "Mai", "Lê", "Mai", "mai@acme.example", true},
		{"accents the header left out", "An Văn", "Nguyễn", "Nguyen Van An", "an.nguyen@example.vn", true},
		{"accents the read left out", "Jose", "Garcia", "José García", "info@example.es", true},
		{"umlauts written out in the address", "Jörg", "Müller", "Info", "mueller@example.de", true},
		{"an initial in the header", "John", "Smith", "J. Smith", "office@example.com", true},
		{"an initial in the address", "John", "Smith", "Sales Team", "j.smith@example.com", true},
		{"a nickname in the header", "Robert", "Smith", "Bob Smith", "office@example.com", true},
		{"a hyphenated given name split in the header", "Anna-Lena", "Schmidt", "Anna Lena Schmidt", "info@example.de", true},
		{"a hyphenated given name kept in the address", "Anna Lena", "Schmidt", "Schmidt", "anna-lena.schmidt@example.de", true},
		{"a compact address", "Marcus", "Greven", "Greven Ebusiness", "mgreven@greven.example", true},
		{"a company header, the address names them", "Marcus", "Greven", "Greven Ebusiness", "marcus.greven@greven.example", true},
		{"another participant", "Alice", "Example", "Bob", "bob@example.org", false},
		{"another participant whose family name is the header", "Alice", "Example", "Example", "info@example.org", false},
		{"another participant, header was the address", "Alice", "Example", "bob@example.org", "bob@example.org", false},
		{"a colleague sharing only the surname", "Ann", "Jones", "Bob Jones", "bob.jones@example.org", false},
		{"a colleague sharing the first name", "Anna", "Jones", "Anna Weber", "anna.weber@example.org", false},
		{"no family name read", "Anna", "", "Anna Weber", "anna.weber@example.org", false},
	} {
		if got := NameIsContacts(c.given, c.family, c.fullName, c.email); got != c.want {
			t.Errorf("%s: NameIsContacts(%q, %q) = %v, want %v", c.name, c.given, c.family, got, c.want)
		}
	}
}
