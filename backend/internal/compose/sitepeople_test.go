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

	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
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
