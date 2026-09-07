// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Attribution on a group imprint: which company a grounded detail belongs to.
//
// The lane's other tests ask whether a value is printed on the page. These ask
// whose it is — a distinction with no symptom, because on the page that
// produces it both halves are printed and neither is invented.

import (
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// groupImprint is the common German shape: a parent and a subsidiary, each
// with its own registered address and register number, in blocks the passage
// packer keeps apart.
//
// Both companies are named on the page, so the census — which reads the whole
// page on purpose — counts them both. That is what makes the attribution the
// only thing standing between a reply and a register number that belongs to
// somebody else.
func groupImprint(t *testing.T) (crawlPage, pageMenu, snippetIndex) {
	t.Helper()
	parent := "Impressum. Nordwerk Holding SE, Deliusstrasse 7, 24114 Kiel, Germany. " +
		"Amtsgericht Kiel HRB 111222. USt-IdNr. DE111222333. " +
		strings.Repeat("Vertreten durch den Vorstand. ", 7)
	subsidiary := "Nordwerk Systems GmbH, Kaistrasse 40, 20359 Hamburg, Germany. " +
		"Amtsgericht Hamburg HRB 333444. USt-IdNr. DE333444555. " +
		strings.Repeat("Geschaeftsfuehrung nach Satzung. ", 7)
	page, menu, idx := pageFixture(crmcontracts.SiteReadPageKindImpressum, seedURL+"/impressum",
		parent+"\n"+subsidiary)
	if len(idx.refs) < 2 {
		t.Fatalf("the two companies must land in separate passages, got %d", len(idx.refs))
	}
	return page, menu, idx
}

// The defect this guards: a reply names one company and cites the other's
// block. Nothing is hallucinated — the address and the register number are
// printed, in full, on the page the entity was found on — so every
// no-guess check downstream passes, and the census hands a human a company
// whose registered identity is its sibling's.
func TestALegalDetailCitedFromASiblingsBlockIsRefused(t *testing.T) {
	page, menu, idx := groupImprint(t)

	// s1 is the subsidiary's block; the reply attributes it to the parent.
	reply := `{"facts":[],"entities":[{"n":"Nordwerk Holding SE",` +
		`"a":"Kaistrasse 40, 20359 Hamburg, Germany","r":"HRB 333444","v":"DE333444555","e":"s1"}]}`
	res, dropped := gatePageEntities2(t, reply, page, menu, idx)

	// The entity itself survives: it is printed on the page, and dropping it
	// would undercount the census, which is the failure the page-wide name
	// check exists to prevent.
	if len(res) != 1 || res[0].Name != "Nordwerk Holding SE" {
		t.Fatalf("the parent is printed on this page and stays in the census: %+v", res)
	}
	if res[0].RegisteredAddress != "" || res[0].RegisterNumber != "" || res[0].VatNumber != "" {
		t.Errorf("the subsidiary's registered identity attached to the parent: %+v", res[0])
	}
	reasons := dropReasons(dropped)
	for _, field := range []string{fieldRegisteredAddress, fieldRegisterNumber, fieldRegisterVat} {
		if reasons[field] != dropLegalBlockNotThisEntity {
			t.Errorf("%s was refused as %q, want %q — the value IS on the page, and reporting it as "+
				"ungrounded sends a reader looking for text that is right there",
				field, reasons[field], dropLegalBlockNotThisEntity)
		}
	}
}

// The same page, cited correctly. The guard has to leave the ordinary answer
// alone, or a multi-entity imprint returns two names and no details at all —
// which is the shape of over-correction that gets a gate turned off.
func TestEachCompanyKeepsTheDetailsPrintedInItsOwnBlock(t *testing.T) {
	page, menu, idx := groupImprint(t)

	reply := `{"facts":[],"entities":[` +
		`{"n":"Nordwerk Holding SE","a":"Deliusstrasse 7, 24114 Kiel, Germany",` +
		`"r":"HRB 111222","v":"DE111222333","e":"s0"},` +
		`{"n":"Nordwerk Systems GmbH","a":"Kaistrasse 40, 20359 Hamburg, Germany",` +
		`"r":"HRB 333444","v":"DE333444555","e":"s1"}]}`
	res, dropped := gatePageEntities2(t, reply, page, menu, idx)

	if len(res) != 2 {
		t.Fatalf("both companies are printed and both stand: %+v", res)
	}
	want := map[string][3]string{
		"Nordwerk Holding SE":   {"Deliusstrasse 7, 24114 Kiel, Germany", "HRB 111222", "DE111222333"},
		"Nordwerk Systems GmbH": {"Kaistrasse 40, 20359 Hamburg, Germany", "HRB 333444", "DE333444555"},
	}
	for _, got := range res {
		w, known := want[got.Name]
		if !known {
			t.Errorf("unexpected entity %q", got.Name)
			continue
		}
		if got.RegisteredAddress != w[0] || got.RegisterNumber != w[1] || got.VatNumber != w[2] {
			t.Errorf("%s kept %q/%q/%q, want %q/%q/%q — a correctly cited detail must survive",
				got.Name, got.RegisteredAddress, got.RegisterNumber, got.VatNumber, w[0], w[1], w[2])
		}
	}
	if len(dropped) != 0 {
		t.Errorf("nothing is refused when each company cites its own block: %+v", dropped)
	}
}

// A citation outside the numbered index yields no block, and now says so.
// Reporting it as a value the page does not print would be false — the page
// was never asked.
func TestACitationOutsideTheIndexIsReportedAsUnknown(t *testing.T) {
	page, menu, idx := groupImprint(t)

	reply := `{"facts":[],"entities":[{"n":"Nordwerk Holding SE",` +
		`"a":"Deliusstrasse 7, 24114 Kiel, Germany","r":"HRB 111222","e":"s99"}]}`
	res, dropped := gatePageEntities2(t, reply, page, menu, idx)

	if len(res) != 1 || res[0].RegisteredAddress != "" || res[0].RegisterNumber != "" {
		t.Fatalf("a detail with no resolvable citation carries no evidence: %+v", res)
	}
	if got := dropReasons(dropped)[fieldRegisteredAddress]; got != dropSnippetIDUnknown {
		t.Errorf("the unresolvable citation was reported as %q, want %q", got, dropSnippetIDUnknown)
	}
}
