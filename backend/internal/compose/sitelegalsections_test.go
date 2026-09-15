// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"fmt"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/aitasks"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/webread"
)

func legalHTMLPage(doc string) (crawlPage, pageMenu, snippetIndex) {
	page := pageFrom(seedURL+"/en/imprint", crmcontracts.SiteReadPageKindImpressum,
		webread.Page{Text: webread.StripTags(doc), Sections: webread.HeadingSections(doc)})
	menu, _ := menuForKind(page.Kind)
	excerpt, _ := pageFactsExcerpt(page)
	return page, menu, newSnippetIndex(excerpt)
}

func TestHeadingSectionsKeepTheSingaporeIdentityTogether(t *testing.T) {
	doc := `<h1>Imprint</h1><h2>Acme Singapore</h2><p>Acme Pte. Ltd.</p>
 <p>77 High Street, #09-11 High Street Plaza, Singapore (179433)</p>
 <p>Contact: info@acme.example Authorized: Alex Example Business Profile: 201629357M</p>
 <h2>Acme Thailand</h2><p>Acme (Thailand) Co., Ltd.</p>
 <p>7 Summer Point Building, Level 2, Suite 12, Bangkok, Thailand 10110</p>
 <p>Contact: info@acme.example Business Profile: 0105563011797</p>
 <h2>Acme Vietnam</h2><p>Acme Vietnam Limited</p>
 <p>Saigon Pearl, Sapphire 2, 92 Nguyen Huu Canh, Ho Chi Minh City, Vietnam</p>
 <p>Contact: info@acme.example Business Profile: 0314588343</p>`
	page, menu, idx := legalHTMLPage(doc)
	reply := `{"facts":[],"entities":[
 {"n":"Acme Pte. Ltd.","a":"77 High Street, #09-11 High Street Plaza, Singapore (179433)","r":"201629357M","e":"s0"},
 {"n":"Acme (Thailand) Co., Ltd.","a":"7 Summer Point Building, Level 2, Suite 12, Bangkok, Thailand 10110","r":"0105563011797","e":"s1"},
 {"n":"Acme Vietnam Limited","a":"Saigon Pearl, Sapphire 2, 92 Nguyen Huu Canh, Ho Chi Minh City, Vietnam","r":"0314588343","e":"s2"}]} `
	entities, dropped := gatePageEntities2(t, reply, page, menu, idx)
	if len(entities) != 3 || len(dropped) != 0 {
		t.Fatalf("entities=%+v drops=%+v", entities, dropped)
	}
	read := contacts.SiteRead{}
	for _, entity := range entities {
		if entity.RegisteredAddress == "" || entity.RegisterNumber == "" {
			t.Errorf("identity lost: %+v", entity)
		}
		read.LegalEntities = append(read.LegalEntities, contacts.SiteReadLegalEntity{Name: entity.Name, RegisteredAddress: entity.RegisteredAddress, SourceURL: entity.SourceURL})
	}
	for _, clarify := range entityClarifies(read, "en") {
		if clarify.Field == fieldRegisteredAddress {
			if len(clarify.Options) != 3 || clarify.Options[0].Value != entities[0].RegisteredAddress {
				t.Fatalf("Singapore missing: %+v", clarify.Options)
			}
			return
		}
	}
	t.Fatal("addresses never reached onboarding confirmation")
}

func TestLegalSubheadingsAndNamePunctuationKeepThePrintedDetails(t *testing.T) {
	for _, tc := range []struct{ name, doc, entity, address, register, vat string }{
		{"subheadings", `<h1>Impressum</h1><p>Acme GmbH, Musterstr. 1, 12345 Berlin</p><h2>Vertreten durch</h2><p>Alex Muster</p><h2>Registereintrag</h2><p>Amtsgericht Berlin HRB 12345</p><h2>Umsatzsteuer-ID</h2><p>DE123456789</p>`, "Acme GmbH", "Musterstr. 1, 12345 Berlin", "HRB 12345", "DE123456789"},
		{"punctuation", `<h2>Acme Singapore</h2><p>Acme Pte. Ltd.</p><p>77 High Street, Singapore (179433)</p><p>201629357M</p>`, "Acme Pte Ltd", "77 High Street, Singapore 179433", "201629357M", ""},
		{"spanish", `<h1>Aviso legal</h1><p>Acme S.L. Rufino González 23 bis, Madrid 28037. Los datos se conservan conforme a la normativa vigente.</p>`, "Acme S.L.", "Rufino González 23 bis, Madrid 28037", "", ""},
		{"shared heading", `<h2>Group imprint</h2><p>North GmbH, First Street 1. HRB 111222. South GmbH, Second Street 2. HRB 333444.</p>`, "North GmbH", "First Street 1", "HRB 111222", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			page, menu, idx := legalHTMLPage(tc.doc)
			reply := fmt.Sprintf(`{"facts":[],"entities":[{"n":%q,"a":%q,"r":%q,"v":%q,"e":"s0"}]}`, tc.entity, tc.address, tc.register, tc.vat)
			entities, drops := gatePageEntities2(t, reply, page, menu, idx)
			if len(entities) != 1 || len(drops) != 0 || entities[0].RegisteredAddress != tc.address || entities[0].RegisterNumber != tc.register || entities[0].VatNumber != tc.vat {
				t.Fatalf("entities=%+v drops=%+v", entities, drops)
			}
		})
	}
}

func TestHeadingPassagesKeepTheExistingSiblingAttributionGuard(t *testing.T) {
	doc := `<h2>North</h2><p>North GmbH, First Street 1. HRB 111222. ` + strings.Repeat("Represented by management. ", 9) + `</p><h2>South</h2><p>South GmbH, Second Street 2. HRB 333444. ` + strings.Repeat("Represented by management. ", 9) + `</p>`
	page, menu, idx := legalHTMLPage(doc)
	for i := range idx.refs {
		reply := fmt.Sprintf(`{"facts":[],"entities":[{"n":"North GmbH","a":"Second Street 2","r":"HRB 333444","e":"s%d"}]}`, i)
		entities, _ := gatePageEntities2(t, reply, page, menu, idx)
		if len(entities) != 1 || entities[0].RegisteredAddress != "" || entities[0].RegisterNumber != "" {
			t.Fatalf("sibling identity accepted: %+v", entities)
		}
	}
}

func TestLegalSectionsKeepEarlierBlocksWhenThePageIsTruncated(t *testing.T) {
	doc := `<h2>Acme</h2><p>Acme Pte. Ltd.</p><p>77 High Street, Singapore 179433</p><h2>Long terms</h2><p>` + strings.Repeat("Legal notice continues. ", 1000) + `</p><h2>Unread</h2><p>Unseen GmbH HRB 999999</p>`
	page, _, idx := legalHTMLPage(doc)
	excerpt, unread := pageFactsExcerpt(page)
	if unread <= 0 || len([]rune(excerpt[0].Text)) != pageFactsExcerptRunes {
		t.Fatal("real excerpt budget was not applied")
	}
	if !strings.Contains(idx.refs[0].passage, "Acme Pte. Ltd. 77 High Street") {
		t.Fatal("a later truncation split the earlier company block")
	}
	for _, ref := range idx.refs {
		if len([]rune(ref.passage)) >= snippetMaxRunes+snippetMinRunes {
			t.Fatal("heading widened the passage limit")
		}
		if strings.Contains(ref.passage, "HRB 999999") {
			t.Fatal("unread section entered the evidence")
		}
	}
}

func TestLegalHeadingRecoveryCannotIntroduceUnprintedText(t *testing.T) {
	for _, doc := range []string{`<p>Old text <img alt="<script"> ignored</p><h2>Acme GmbH</h2><p>Invented Street 1</p>`, `<!-- <h2>Comment</h2> --><textarea><h2>Literal heading</h2></textarea><h2>Acme GmbH</h2><p>First Street 1`} {
		page, _, _ := legalHTMLPage(doc)
		if normalizeEvidence(legalSectionText(page)) != normalizeEvidence(page.Text) {
			t.Fatal("heading recovery changed the source text")
		}
	}
}

func TestCertificationReadsTheSameLegalHeadingEvidence(t *testing.T) {
	doc := `<h2>Acme Singapore</h2><p>Acme Pte. Ltd.</p><p>77 High Street, Singapore 179433</p><p>Business Profile: 201629357M</p>`
	page, _, _ := legalHTMLPage(doc)
	fixture := sitePageFactsJSON(t, sitePageFactsFixture{
		URL: page.URL, Kind: page.Kind, Text: page.Text, Sections: page.Sections,
	})
	reply := `{"facts":[{"f":"location","v":"Singapore","e":"s0"}],"entities":[{"n":"Acme Pte. Ltd.","a":"77 High Street, Singapore 179433","r":"201629357M","e":"s0"}]}`
	outcome, trace := runSitePageFactsCase(t, fixture, map[string]string{contacts.FactLocation: "Singapore"}, reply)
	if outcome.Result != aitasks.OutcomeAccepted || strings.Contains(outcome.Detail, "legal_block_not_this_entity") {
		t.Fatalf("certification lost the legal identity: %+v", outcome)
	}
	if len(trace.Requests) != 1 || !strings.Contains(trace.Requests[0].Messages[0].Content, "Acme Pte. Ltd. 77 High Street") {
		t.Fatal("certification did not use the production heading-aware excerpt")
	}
}
