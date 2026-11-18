// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package salutation

import "testing"

const aliceSignature = "Best regards\nAlice Example\nFounder\n+49 30 5550100\n"

func TestNamesSignerTakesTheAddressAloneOnlyWhenNoOtherSignerShows(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name, block, fullName string
		want                  bool
	}{
		{"their name", "Viele Grüße\nJudith Andresen\nGeschäftsführerin", "Judith Andresen", true},
		{"their address and nothing else", "Viele Grüße\njudith@example.test", "Judith Andresen", true},
		{"their address beside another signer's", aliceSignature + "alice@example.com\nbob@example.org", "Bob", false},
		{"their name inside their own address", aliceSignature + "alice@example.com\nbob@example.org", "Bob Contact", false},
		{"their name beside a colleague's address", "Best\nBob Contact\nbob@example.org\nteam@example.org", "Bob Contact", true},
		{"a surname alone", "Best\nAnn Jones\nCEO", "Ann Smith", false},
	} {
		email := "judith@example.test"
		if c.fullName != "Judith Andresen" {
			email = "bob@example.org"
		}
		if got := NamesSigner(c.block, c.fullName, email); got != c.want {
			t.Errorf("%s: NamesSigner = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestNameIsContactsRefusesAnotherParticipantsName(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name, read, fullName, email string
		want                        bool
	}{
		{"the header name in printed order", "Lê Thị Mai", "Mai", "mai@acme.example", true},
		{"the same two names", "Bob Contact", "Bob Contact", "bob@example.org", true},
		{"a company header, the address names them", "Marcus Greven", "Greven Ebusiness", "marcus.greven@greven.example", true},
		{"a compact address", "Marcus Greven", "Greven Ebusiness", "mgreven@greven.example", true},
		{"another participant", "Alice Example", "Bob", "bob@example.org", false},
		{"another participant, header was the address", "Alice Example", "bob@example.org", "bob@example.org", false},
		{"a colleague sharing only the surname", "Ann Jones", "Ann Smith", "ann.smith@example.org", false},
		{"a short name inside a longer address", "Ann Brown", "Joanne", "joanne@example.org", false},
		{"a colleague sharing the first name", "Anna Jones", "Anna Weber", "anna.weber@example.org", false},
	} {
		if got := NameIsContacts(c.read, c.fullName, c.email); got != c.want {
			t.Errorf("%s: NameIsContacts(%q) = %v, want %v", c.name, c.read, got, c.want)
		}
	}
}
