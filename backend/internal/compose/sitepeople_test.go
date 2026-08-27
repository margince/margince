// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The site-lead identity contract: the cross-read natural key is org +
// normalized name (+ published email), stable across page moves and
// reflow. The published-only people GATE rules live with the corpus gate
// (sitecorpusread_test.go).

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestSiteLeadSourceIDIsOrgStableAcrossPagesAndNameReflow(t *testing.T) {
	org := ids.NewV7()
	// The key is the ORG + name, not the page: the same person found on
	// /team or /about, or after a re-crawl moved the page, is one lead.
	teamPage := siteLeadSourceID(org, "Anna Muster", "")
	aboutPage := siteLeadSourceID(org, "  anna   MUSTER ", "")
	if teamPage != aboutPage {
		t.Fatal("the lead natural key changed on a whitespace/case reflow, or across pages of the same site")
	}
	// A different org is a different lead even for the same name.
	if teamPage == siteLeadSourceID(ids.NewV7(), "Anna Muster", "") {
		t.Fatal("the same name at two organizations collapsed to one lead key")
	}
	// Two distinct people who share a name stay distinct via published email.
	if siteLeadSourceID(org, "Anna Muster", "anna1@acme.example") ==
		siteLeadSourceID(org, "Anna Muster", "anna2@acme.example") {
		t.Fatal("two people sharing a name but not an email share one key")
	}
	if teamPage == siteLeadSourceID(org, "Bernd Beispiel", "") {
		t.Fatal("two different people share one lead natural key")
	}
	if strings.Contains(teamPage, "@") || len(teamPage) != 64 {
		t.Fatalf("source id = %q, want a bare sha256 hex digest (no PII in the key)", teamPage)
	}
}

// The two dimensions a person key has to fold, kept together because a key
// that has one and not the other still mints a duplicate lead — and each was
// held by only one of the two normalizers this key used to be spelled with.
func TestSiteLeadSourceIDFoldsTheDACHPairAndReflowedWhitespace(t *testing.T) {
	org := ids.NewV7()
	for _, c := range []struct {
		what  string
		left  string
		right string
	}{
		// strings.ToLower leaves ß alone, so a casefold-only key mints a
		// second lead for the pair the DACH market prints daily.
		{"a full Unicode fold", "Straße Müller", "STRASSE MULLER"},
		// A page that reflows a name across a line break prints the same
		// person; a key that keeps the second space mints them again.
		{"an internal-whitespace collapse", "Anna Muster", "Anna  Muster"},
	} {
		if siteLeadSourceID(org, c.left, "") != siteLeadSourceID(org, c.right, "") {
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
// consulted, and one of the two published people was dropped. The lead key
// could not put back a person the merge had already discarded.
func TestSitePersonIdentityKeepsTwoPeopleTheAddressesTellApart(t *testing.T) {
	// The person key unaccents, which is what makes one person spelled two
	// ways one person — and exactly why the address has to be part of this.
	if people.NormalizePersonName("José Silva") != people.NormalizePersonName("Jose Silva") {
		t.Fatal("the person key stopped unaccenting, so this test no longer plants the case it was written for")
	}
	if sitePersonIdentity("José Silva", "jose@acme.example") ==
		sitePersonIdentity("Jose Silva", "silva@acme.example") {
		t.Error("two published people with different printed addresses folded into one — the page lane " +
			"discards one of them before the lead key can tell them apart")
	}
	// One person, two spellings of their name, one address: still one person.
	if sitePersonIdentity("José Silva", "jose@acme.example") !=
		sitePersonIdentity("Jose Silva", "  JOSE@acme.example ") {
		t.Error("one person listed twice under two spellings took two identities")
	}
	// And it is the SAME identity the cross-read lead key is built on, so the
	// two cannot decide differently about one pair.
	org := ids.NewV7()
	if (siteLeadSourceID(org, "José Silva", "jose@acme.example") ==
		siteLeadSourceID(org, "Jose Silva", "silva@acme.example")) !=
		(sitePersonIdentity("José Silva", "jose@acme.example") ==
			sitePersonIdentity("Jose Silva", "silva@acme.example")) {
		t.Error("the lead key and the read's own fold disagree about one pair")
	}
}
