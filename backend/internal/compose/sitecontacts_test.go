// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The site-lead identity contract: the cross-read natural key is company +
// normalized name (+ published email), stable across page moves and
// reflow. The published-only contacts GATE rules live with the corpus gate
// (sitecorpusread_test.go).

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestSiteLeadSourceIDIsCompanyStableAcrossPagesAndNameReflow(t *testing.T) {
	company := ids.NewV7()
	// The key is the COMPANY + name, not the page: the same contact found on
	// /team or /about, or after a re-crawl moved the page, is one lead.
	teamPage := siteLeadSourceID(company, "Anna Muster", "")
	aboutPage := siteLeadSourceID(company, "  anna   MUSTER ", "")
	if teamPage != aboutPage {
		t.Fatal("the lead natural key changed on a whitespace/case reflow, or across pages of the same site")
	}
	// A different company is a different lead even for the same name.
	if teamPage == siteLeadSourceID(ids.NewV7(), "Anna Muster", "") {
		t.Fatal("the same name at two companies collapsed to one lead key")
	}
	// Two distinct contacts who share a name stay distinct via published email.
	if siteLeadSourceID(company, "Anna Muster", "anna1@acme.example") ==
		siteLeadSourceID(company, "Anna Muster", "anna2@acme.example") {
		t.Fatal("two contacts sharing a name but not an email share one key")
	}
	if teamPage == siteLeadSourceID(company, "Bernd Beispiel", "") {
		t.Fatal("two different contacts share one lead natural key")
	}
	if strings.Contains(teamPage, "@") || len(teamPage) != 64 {
		t.Fatalf("source id = %q, want a bare sha256 hex digest (no PII in the key)", teamPage)
	}
}

// The two dimensions a contact key has to fold, kept together because a key
// that has one and not the other still mints a duplicate lead — and each was
// held by only one of the two normalizers this key used to be spelled with.
func TestSiteLeadSourceIDFoldsTheDACHPairAndReflowedWhitespace(t *testing.T) {
	company := ids.NewV7()
	for _, c := range []struct {
		what  string
		left  string
		right string
	}{
		// strings.ToLower leaves ß alone, so a casefold-only key mints a
		// second lead for the pair the DACH market prints daily.
		{"a full Unicode fold", "Straße Müller", "STRASSE MULLER"},
		// A page that reflows a name across a line break prints the same
		// contact; a key that keeps the second space mints them again.
		{"an internal-whitespace collapse", "Anna Muster", "Anna  Muster"},
	} {
		if siteLeadSourceID(company, c.left, "") != siteLeadSourceID(company, c.right, "") {
			t.Errorf("%q and %q took two lead keys — the key lost %s", c.left, c.right, c.what)
		}
	}
}

// The identity every step of a site read folds on, and the pair that made it
// have to be more than a name.
//
// The page lane and the cross-page merge used to fold on the NAME alone while
// the lead key folded on name plus the printed address. Two keys, and the
// looser one ran first: two colleagues whose names differ only by an accent
// were folded into one before the address that tells them apart was ever
// consulted, and one of the two published contacts was dropped. The lead key
// could not put back a contact the merge had already discarded.
func TestSiteContactIdentityKeepsTwoContactsTheAddressesTellApart(t *testing.T) {
	// The contact key unaccents, which is what makes one contact spelled two
	// ways one contact — and exactly why the address has to be part of this.
	if contacts.NormalizeContactName("José Silva") != contacts.NormalizeContactName("Jose Silva") {
		t.Fatal("the contact key stopped unaccenting, so this test no longer plants the case it was written for")
	}
	if siteContactIdentity("José Silva", "jose@acme.example") ==
		siteContactIdentity("Jose Silva", "silva@acme.example") {
		t.Error("two published contacts with different printed addresses folded into one — the page lane " +
			"discards one of them before the lead key can tell them apart")
	}
	// One contact, two spellings of their name, one address: still one contact.
	if siteContactIdentity("José Silva", "jose@acme.example") !=
		siteContactIdentity("Jose Silva", "  JOSE@acme.example ") {
		t.Error("one contact listed twice under two spellings took two identities")
	}
	// And it is the SAME identity the cross-read lead key is built on, so the
	// two cannot decide differently about one pair.
	company := ids.NewV7()
	if (siteLeadSourceID(company, "José Silva", "jose@acme.example") ==
		siteLeadSourceID(company, "Jose Silva", "silva@acme.example")) !=
		(siteContactIdentity("José Silva", "jose@acme.example") ==
			siteContactIdentity("Jose Silva", "silva@acme.example")) {
		t.Error("the lead key and the read's own fold disagree about one pair")
	}
}
