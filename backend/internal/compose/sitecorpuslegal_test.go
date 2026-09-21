// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// A group's legal notice states the same entity more than once: every
// locale of the page repeats it, and each block is headed by the market
// it trades in. The census a human is offered must be the companies, not
// the sightings.
func TestDedupeLegalEntitiesFoldsOnTheRegisterNumber(t *testing.T) {
	entities := []corpusLegalEntity{
		{Name: "Acme Pte. Ltd.", RegisterNumber: "201629357M", SourceURL: seedURL + "/imprint"},
		// The market heading printed above the block, which the page
		// states as prominently as the entity itself.
		{Name: "Acme Singapore", RegisterNumber: "201629357M", SourceURL: seedURL + "/imprint"},
		// The German locale of the same page, this time with the address.
		{
			Name: "Acme Pte. Ltd.", RegisteredAddress: "77 High Street, Singapore",
			RegisterNumber: "201629357M", SourceURL: seedURL + "/de/imprint",
		},
		// Another locale can lose the register number at a passage boundary.
		// Its matching name must not create a duplicate choice for the human.
		{Name: "Acme Pte. Ltd.", SourceURL: seedURL + "/th/imprint"},
		{Name: "Acme GmbH", RegisterNumber: "HRB 12345", SourceURL: seedURL + "/imprint"},
	}
	got := dedupeLegalEntities(entities)
	if len(got) != 2 {
		t.Fatalf("four sightings of two companies must fold to two: %+v", got)
	}
	if got[0].RegisteredAddress != "77 High Street, Singapore" {
		t.Errorf("the richest sighting must win, so a locale that printed the address is not lost: %+v", got[0])
	}
	if got[1].Name != "Acme GmbH" {
		t.Errorf("a distinct register number is a distinct company: %+v", got[1])
	}
}

func TestDedupeLegalEntitiesKeepsSameNameWithDistinctRegisters(t *testing.T) {
	entities := []corpusLegalEntity{
		{Name: "Acme Ltd", RegisterNumber: "SG-1", SourceURL: seedURL + "/imprint"},
		{Name: "Acme Ltd", RegisterNumber: "UK-2", SourceURL: seedURL + "/en/imprint"},
	}
	if got := dedupeLegalEntities(entities); len(got) != 2 {
		t.Fatalf("different registry identities must remain separate: %+v", got)
	}
}

func TestDedupeLegalEntitiesFoldsPunctuationVariantsAndDropsBareBrand(t *testing.T) {
	entities := []corpusLegalEntity{
		{Name: "RealtimeBoard, Inc. dba Miro", RegisteredAddress: "San Francisco", SourceURL: seedURL + "/legal"},
		{Name: "RealtimeBoard Inc dba Miro", SourceURL: seedURL + "/imprint"},
		{Name: "RealtimeBoard B.V.", RegisterNumber: "123", SourceURL: seedURL + "/legal"},
		{Name: "RealtimeBoard BV", RegisterNumber: "123", SourceURL: seedURL + "/imprint"},
		{Name: "Miro", SourceURL: seedURL + "/legal"},
	}
	got := dedupeLegalEntities(entities)
	if len(got) != 2 {
		t.Fatalf("punctuation variants and a bare brand must not become legal choices: %+v", got)
	}
	if got[0].RegisteredAddress != "San Francisco" || got[1].RegisterNumber != "123" {
		t.Fatalf("the richest registered sightings must survive: %+v", got)
	}
}

func TestDedupeLegalEntitiesKeepsTheOnlyBareLegalName(t *testing.T) {
	got := dedupeLegalEntities([]corpusLegalEntity{{Name: "Miro", SourceURL: seedURL + "/legal"}})
	if len(got) != 1 {
		t.Fatalf("an unusual legal name must survive when no richer registered alias exists: %+v", got)
	}
}

func TestEnrichSingleLegalEntityFromGatedProfile(t *testing.T) {
	entities := []corpusLegalEntity{{Name: "Acme GmbH", SourceURL: seedURL + "/imprint"}}
	// Each number fills its OWN field. A register entry recovered by the
	// profile lane cannot stand in for a VAT ID the page never printed, and
	// the two authorities behind them are why.
	fields := []evidencedField{
		{Field: "registered_address", Value: "Deliusstrasse 7, 24114 Kiel"},
		{Field: "register_number", Value: "HRB 123456"},
		{Field: "register_vat", Value: "DE123456789"},
	}
	got := enrichLegalEntitiesFromProfile(entities, fields)
	if got[0].RegisteredAddress == "" || got[0].RegisterNumber != "HRB 123456" || got[0].VatNumber != "DE123456789" {
		t.Fatalf("the single legal choice must reuse the already-gated fields: %+v", got)
	}
	if entities[0].RegisteredAddress != "" {
		t.Fatal("enrichment must not mutate the source slice")
	}
	if many := enrichLegalEntitiesFromProfile(append(entities, corpusLegalEntity{Name: "Acme Inc."}), fields); many[0].RegisterNumber != "" {
		t.Fatal("profile values must never be assigned across a multi-entity census")
	}
}

// Without a register number there is nothing authoritative to fold on, so
// the name is the identity — two genuinely different names stay two.
func TestDedupeLegalEntitiesFallsBackToTheNameWithoutARegisterNumber(t *testing.T) {
	entities := []corpusLegalEntity{
		{Name: "Acme GmbH", SourceURL: seedURL + "/imprint"},
		{Name: "Acme GmbH", RegisteredAddress: "Kiel", SourceURL: seedURL + "/de/imprint"},
		{Name: "Acme Ltd", SourceURL: seedURL + "/imprint"},
	}
	got := dedupeLegalEntities(entities)
	if len(got) != 2 {
		t.Fatalf("two names must survive as two entities: %+v", got)
	}
	if got[0].RegisteredAddress != "Kiel" {
		t.Errorf("the sighting that carried the address must win: %+v", got[0])
	}
}

func TestLegalEntityDetailCountsWhatWasPrinted(t *testing.T) {
	for _, tc := range []struct {
		name   string
		entity corpusLegalEntity
		want   int
	}{
		{"name only", corpusLegalEntity{Name: "Acme GmbH"}, 0},
		{"with address", corpusLegalEntity{Name: "Acme GmbH", RegisteredAddress: "Kiel"}, 1},
		{"with a register entry", corpusLegalEntity{Name: "Acme GmbH", RegisteredAddress: "Kiel", RegisterNumber: "HRB 1"}, 2},
		{"the whole block", corpusLegalEntity{
			Name: "Acme GmbH", RegisteredAddress: "Kiel", RegisterNumber: "HRB 1", VatNumber: "DE1",
		}, 3},
		{"a VAT ID counts like the register entry beside it", corpusLegalEntity{Name: "Acme GmbH", VatNumber: "DE1"}, 1},
		{"blank is not printed", corpusLegalEntity{Name: "Acme GmbH", RegisteredAddress: "  "}, 0},
	} {
		if got := legalEntityDetail(tc.entity); got != tc.want {
			t.Errorf("%s: legalEntityDetail = %d, want %d", tc.name, got, tc.want)
		}
	}
}

// The trio is withheld for two unrelated reasons, and the human is told
// which. A run whose legal page failed to extract must never be told the
// domain hosts several companies — that is a corporate structure nobody
// read, stated as fact on the strength of a provider outage.
func TestLegalAbstentionNamesTheCauseThatFired(t *testing.T) {
	one := []corpusLegalEntity{{Name: "Acme GmbH", SourceURL: seedURL + "/impressum"}}
	two := []corpusLegalEntity{
		{Name: "Acme GmbH", SourceURL: seedURL + "/impressum"},
		{Name: "Acme Pte. Ltd.", SourceURL: seedURL + "/impressum"},
	}
	for _, tc := range []struct {
		name             string
		entities         []corpusLegalEntity
		censusIncomplete bool
		want             legalAbstention
	}{
		{"a legal page that never came back", one, true, legalAbstentionCensusIncomplete},
		{"a domain that states two entities", two, false, legalAbstentionMultipleEntities},
		{"both: what the site publishes outranks what this run missed", two, true, legalAbstentionMultipleEntities},
		{"a settled census", one, false, legalAbstentionNone},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := legalAbstentionOf(tc.entities, tc.censusIncomplete)
			if got != tc.want {
				t.Fatalf("abstention = %q, want %q", got, tc.want)
			}
			// The drop record and the sentence a human reads answer the
			// same question, so they are derived from the same value.
			if want := tc.want.warning(); got.warning() != want {
				t.Errorf("warning = %q, want %q", got.warning(), want)
			}
		})
	}
	if legalAbstentionCensusIncomplete.warning() == legalAbstentionMultipleEntities.warning() {
		t.Fatal("the two causes must not share one sentence")
	}
	if strings.Contains(legalWarningCensusIncomplete, "more than one entity") {
		t.Errorf("the incomplete-census warning must claim nothing about the number of entities: %q", legalWarningCensusIncomplete)
	}
}

// TestCensusFillsTheAddressTheProfileLaneMissed is the fix for the gap that
// left 136 of the demo dataset's 190 companies with no address at all.
//
// The two legal lanes read the same page and disagree. The census reads the
// whole page; the profile lane reads a bounded excerpt and gates against the
// single passage the model cited. communicode.de's census carried
// "Wittekindstr. 1a, 45131 Essen" while its profile lane dropped the same
// address for citing a neighbouring passage, so the field never existed.
func TestCensusFillsTheAddressTheProfileLaneMissed(t *testing.T) {
	const impressum = "https://example.com/impressum"
	kinds := map[string]crmcontracts.SiteReadPageKind{
		impressum: crmcontracts.SiteReadPageKindImpressum,
	}
	entities := []corpusLegalEntity{{
		Name:              "communicode GmbH",
		RegisteredAddress: "Wittekindstr. 1a, 45131 Essen",
		RegisterNumber:    "HRB 37643",
		VatNumber:         "DE216235279",
		EvidenceSnippet:   "Impressum communicode GmbH, Wittekindstr. 1a in 45131 Essen",
		SourceURL:         impressum,
	}}
	// The profile lane produced only the display name: its legal trio was
	// dropped, which is the real state this replays.
	fields := []evidencedField{{Field: "display_name", Value: "communicode", SourceURL: impressum, Confidence: 1}}

	out := fillLegalTrioFromCensus(fields, entities, kinds, false)
	got := map[string]string{}
	for _, f := range out {
		got[f.Field] = f.Value
	}
	if got["registered_address"] != "Wittekindstr. 1a, 45131 Essen" {
		t.Errorf("registered_address = %q, want the address the census proved", got["registered_address"])
	}
	if got["legal_name"] != "communicode GmbH" {
		t.Errorf("legal_name = %q", got["legal_name"])
	}
	// The court's entry and the tax office's number reach their own fields.
	// One field for both meant an imprint printing an HRB number filed it as
	// a VAT ID, and the field added for it stayed empty.
	if got["register_number"] != "HRB 37643" {
		t.Errorf("register_number = %q, want the register entry the census proved", got["register_number"])
	}
	if got["register_vat"] != "DE216235279" {
		t.Errorf("register_vat = %q, want the VAT ID and not the register entry", got["register_vat"])
	}
	// Every filled field must carry the evidence and the page, so the legal
	// gate can judge it exactly as it judges a profile-lane field.
	for _, f := range out {
		if f.Field == "display_name" {
			continue
		}
		if f.EvidenceSnippet == "" || f.SourceURL != impressum {
			t.Errorf("%s arrived without evidence or a source page", f.Field)
		}
	}
}

// A VAT ID is an identity when no register number is printed.
//
// A tax office issues one per company, so two sightings sharing one are the
// same company however their locales name it, and two carrying different ones
// are two companies however alike their names read. Without this a notice
// printing only VAT IDs had no identity at all: one entity split under its
// locale names and could trip the multi-entity abstention, and two companies
// under one name merged into whichever the crawl saw first.
func TestAVatNumberIsAnIdentityWhereNoRegisterNumberIs(t *testing.T) {
	const imprint = "https://example.com/impressum"
	same := dedupeLegalEntities([]corpusLegalEntity{
		{Name: "Acme GmbH", VatNumber: "DE111111111", SourceURL: imprint},
		{Name: "Acme Deutschland", VatNumber: "DE111111111", SourceURL: imprint},
	})
	if len(same) != 1 {
		t.Errorf("one VAT ID under two locale names folded to %d entities, want 1: %+v", len(same), same)
	}

	distinct := dedupeLegalEntities([]corpusLegalEntity{
		{Name: "Acme GmbH", VatNumber: "DE111111111", SourceURL: imprint},
		{Name: "Acme GmbH", VatNumber: "DE222222222", SourceURL: imprint},
	})
	if len(distinct) != 2 {
		t.Errorf("two VAT IDs under one name folded to %d entities, want 2: %+v", len(distinct), distinct)
	}
}

// Two sightings of one entity on one page keep BOTH numbers.
//
// An imprint block naming the address and the VAT ID and another naming the
// address and the register entry tie on how much each states, so a rule that
// kept the richer sighting whole kept whichever came first and dropped the
// other's number. They are three facts about one company and the page order
// decides nothing.
func TestOneEntitySeenTwiceKeepsBothItsNumbers(t *testing.T) {
	const imprint = "https://example.com/impressum"
	got := dedupeLegalEntities([]corpusLegalEntity{
		{Name: "Acme GmbH", RegisteredAddress: "Werkstr. 1", VatNumber: "DE111111111", SourceURL: imprint},
		{Name: "Acme GmbH", RegisteredAddress: "Werkstr. 1", RegisterNumber: "HRB 42", SourceURL: imprint},
	})
	if len(got) != 1 {
		t.Fatalf("one entity seen twice folded to %d, want 1: %+v", len(got), got)
	}
	if got[0].RegisterNumber != "HRB 42" || got[0].VatNumber != "DE111111111" {
		t.Errorf("the fold dropped a number: %+v", got[0])
	}
}

// A detail is never borrowed across pages, because the evidence would lie.
//
// An entity carries ONE snippet and one source URL, and every field filled
// from it is published citing them. A number taken from another page would
// arrive quoting a passage that never printed it — the claim the no-guess
// gate exists to refuse.
func TestADetailIsNeverBorrowedFromAnotherPage(t *testing.T) {
	got := dedupeLegalEntities([]corpusLegalEntity{
		{
			Name: "Acme GmbH", RegisteredAddress: "Werkstr. 1", RegisterNumber: "HRB 42",
			EvidenceSnippet: "Acme GmbH, Werkstr. 1, HRB 42", SourceURL: "https://example.com/impressum",
		},
		{
			Name: "Acme GmbH", VatNumber: "DE111111111",
			EvidenceSnippet: "Acme GmbH, USt-IdNr. DE111111111", SourceURL: "https://example.com/en/imprint",
		},
	})
	if len(got) != 1 {
		t.Fatalf("one entity across two locales folded to %d, want 1: %+v", len(got), got)
	}
	if got[0].VatNumber != "" {
		t.Errorf("a VAT ID from another page rode in on this page's evidence: %+v", got[0])
	}
}

// A register entry never lands in the VAT field, and a VAT ID never lands in
// the register field.
//
// The two numbers come from different authorities — a court issues the
// register entry, a tax office the VAT ID — and a company states both. While
// the census carried one identifier, an imprint printing an HRB number filed
// it as the VAT ID, so register_number stayed empty on every read and an
// accepted refresh could overwrite a real VAT ID with a court entry.
//
// The case is stated in both directions on purpose: an implementation that
// simply swapped the two destinations would pass a test that checked one.
func TestEachLegalNumberReachesItsOwnField(t *testing.T) {
	const impressum = "https://example.com/impressum"
	kinds := map[string]crmcontracts.SiteReadPageKind{impressum: crmcontracts.SiteReadPageKindImpressum}
	entities := []corpusLegalEntity{{
		Name:            "Acme GmbH",
		RegisterNumber:  "HRB 12345 B",
		VatNumber:       "DE123456789",
		EvidenceSnippet: "Acme GmbH, HRB 12345 B, USt-IdNr. DE123456789",
		SourceURL:       impressum,
	}}

	got := map[string]string{}
	for _, f := range fillLegalTrioFromCensus(nil, entities, kinds, false) {
		got[f.Field] = f.Value
	}
	if got["register_number"] != "HRB 12345 B" {
		t.Errorf("register_number = %q, want the court's entry", got["register_number"])
	}
	if got["register_vat"] != "DE123456789" {
		t.Errorf("register_vat = %q, want the tax number", got["register_vat"])
	}
}

// TestCensusFillNeverOverwritesTheProfileLane — a field the profile lane
// produced and the gate kept is the more specific answer and stands.
func TestCensusFillNeverOverwritesTheProfileLane(t *testing.T) {
	const impressum = "https://example.com/impressum"
	kinds := map[string]crmcontracts.SiteReadPageKind{impressum: crmcontracts.SiteReadPageKindImpressum}
	entities := []corpusLegalEntity{{
		Name: "Census GmbH", RegisteredAddress: "Census Weg 1 12345 Ort",
		EvidenceSnippet: "Census GmbH Census Weg 1 12345 Ort", SourceURL: impressum,
	}}
	fields := []evidencedField{{
		Field: "registered_address", Value: "Profile Strasse 2 54321 Stadt",
		EvidenceSnippet: "Profile Strasse 2 54321 Stadt", SourceURL: impressum, Confidence: 0.9,
	}}
	out := fillLegalTrioFromCensus(fields, entities, kinds, false)
	count := 0
	for _, f := range out {
		if f.Field != "registered_address" {
			continue
		}
		count++
		if f.Value != "Profile Strasse 2 54321 Stadt" {
			t.Errorf("the profile lane's value was replaced by %q", f.Value)
		}
	}
	if count != 1 {
		t.Errorf("registered_address appears %d times, want exactly one", count)
	}
}

// TestCensusFillRespectsTheAbstentionAndTheAuthorityRule pins the two ways
// this must refuse. Filling from a census the gate just judged untrustworthy
// would put back exactly what the abstention withheld, and a sighting from a
// page that is not a legal notice must not speak for legal identity.
func TestCensusFillRespectsTheAbstentionAndTheAuthorityRule(t *testing.T) {
	const impressum = "https://example.com/impressum"
	entity := corpusLegalEntity{
		Name: "Anything GmbH", RegisteredAddress: "Irgendwo 1 11111 Ort",
		EvidenceSnippet: "Anything GmbH Irgendwo 1 11111 Ort", SourceURL: impressum,
	}
	legalKinds := map[string]crmcontracts.SiteReadPageKind{impressum: crmcontracts.SiteReadPageKindImpressum}

	if got := fillLegalTrioFromCensus(nil, []corpusLegalEntity{entity}, legalKinds, true); len(got) != 0 {
		t.Error("an abstained read was filled from the census it abstained on")
	}
	two := []corpusLegalEntity{entity, {Name: "Other GmbH", SourceURL: impressum}}
	if got := fillLegalTrioFromCensus(nil, two, legalKinds, false); len(got) != 0 {
		t.Error("an unsettled census with two entities was used to fill legal identity")
	}
	// A contact page is not legal authority, however plainly it prints an address.
	contactOnly := map[string]crmcontracts.SiteReadPageKind{
		impressum: crmcontracts.SiteReadPageKindContact,
	}
	if got := fillLegalTrioFromCensus(nil, []corpusLegalEntity{entity}, contactOnly, false); len(got) != 0 {
		t.Error("a non-legal page was allowed to state the company's legal identity")
	}
	// And a deep path is content ABOUT a legal page, not one.
	deep := entity
	deep.SourceURL = "https://example.com/a/b/c/impressum"
	deepKinds := map[string]crmcontracts.SiteReadPageKind{deep.SourceURL: crmcontracts.SiteReadPageKindImpressum}
	if got := fillLegalTrioFromCensus(nil, []corpusLegalEntity{deep}, deepKinds, false); len(got) != 0 {
		t.Error("a deep-path legal page was treated as the company's own")
	}
}

// TestCensusFillAddsNothingWhenTheCensusHasNothing — an entity stating a name
// and no details fills only the name; the absent details stay absent rather
// than becoming empty fields.
func TestCensusFillAddsNothingWhenTheCensusHasNothing(t *testing.T) {
	const impressum = "https://example.com/impressum"
	kinds := map[string]crmcontracts.SiteReadPageKind{impressum: crmcontracts.SiteReadPageKindImpressum}
	entities := []corpusLegalEntity{{Name: "Bare GmbH", SourceURL: impressum, EvidenceSnippet: "Bare GmbH"}}
	out := fillLegalTrioFromCensus(nil, entities, kinds, false)
	if len(out) != 1 || out[0].Field != "legal_name" {
		t.Fatalf("want just the legal name, got %v", out)
	}
}

// TestCensusFillIsDeterministic pins the field order.
//
// These fields reach the proposal whose JSON is hashed, so iterating a Go map
// — which the first version of this did — gave identical evidence a different
// hash on each run.
func TestCensusFillIsDeterministic(t *testing.T) {
	const impressum = "https://example.com/impressum"
	kinds := map[string]crmcontracts.SiteReadPageKind{impressum: crmcontracts.SiteReadPageKindImpressum}
	entities := []corpusLegalEntity{{
		Name: "Order GmbH", RegisteredAddress: "Weg 1 11111 Ort", RegisterNumber: "HRB 999",
		VatNumber:       "DE999999999",
		EvidenceSnippet: "Order GmbH Weg 1 11111 Ort HRB 999 DE999999999", SourceURL: impressum,
	}}
	want := []string{"legal_name", "registered_address", "register_number", "register_vat"}
	for range 20 {
		out := fillLegalTrioFromCensus(nil, entities, kinds, false)
		if len(out) != len(want) {
			t.Fatalf("got %d fields, want %d", len(out), len(want))
		}
		for i, field := range want {
			if out[i].Field != field {
				t.Fatalf("field %d is %q, want %q — the order is not stable", i, out[i].Field, field)
			}
		}
	}
}

// The census the abstention exists for: one printed name against two
// registrations, stated across the locales of one site.
//
// A sighting that printed only an address contradicts nothing, so it accepted
// the first registration to come along and then stood as the fold target for
// the second. Two registry identities became one row carrying NEITHER number,
// and the settled census that followed offered a human a company whose
// registered identity was assembled from two.
//
// Across LOCALES, because within one page the sightings complete each other
// (fillLegalDetailsFrom) and the anchor stops being numberless after the
// first fold. Nothing completes a sighting across pages — an entity cites one
// snippet, and a number borrowed from another page would be published quoting
// a passage that never printed it.
func TestOneNameAgainstTwoRegistrationsStaysTwoEntities(t *testing.T) {
	entities := []corpusLegalEntity{
		{Name: "Acme GmbH", RegisteredAddress: "Weg 1 11111 Ort", SourceURL: seedURL + "/impressum"},
		{Name: "Acme GmbH", RegisterNumber: "HRB 111", SourceURL: seedURL + "/de/impressum"},
		{Name: "Acme GmbH", RegisterNumber: "HRB 222", SourceURL: seedURL + "/en/imprint"},
	}
	got := dedupeLegalEntities(entities)
	if len(got) < 2 {
		t.Fatalf("two registry identities folded into one company: %+v", got)
	}
	registers := map[string]bool{}
	for _, entity := range got {
		registers[entity.RegisterNumber] = true
	}
	for _, want := range []string{"HRB 111", "HRB 222"} {
		if !registers[want] {
			t.Errorf("%s is gone from the census; the abstention can only refuse what it can see: %+v", want, got)
		}
	}
	if abstention := legalAbstentionOf(got, false); abstention != legalAbstentionMultipleEntities {
		t.Errorf("abstention = %q, want %q — this is the census that must not be answered without a human",
			abstention, legalAbstentionMultipleEntities)
	}
}

// The same shape one authority weaker. A page that prints tax identifiers and
// no register entries has to reach the same verdict, or the guard is about the
// column it was written against rather than about the identity.
func TestOneNameAgainstTwoVatIdentifiersStaysTwoEntities(t *testing.T) {
	entities := []corpusLegalEntity{
		{Name: "Acme GmbH", RegisteredAddress: "Weg 1 11111 Ort", SourceURL: seedURL + "/impressum"},
		{Name: "Acme GmbH", VatNumber: "DE111111111", SourceURL: seedURL + "/de/impressum"},
		{Name: "Acme GmbH", VatNumber: "DE222222222", SourceURL: seedURL + "/en/imprint"},
	}
	got := dedupeLegalEntities(entities)
	if len(got) < 2 {
		t.Fatalf("two tax identities folded into one company: %+v", got)
	}
	if abstention := legalAbstentionOf(got, false); abstention != legalAbstentionMultipleEntities {
		t.Errorf("abstention = %q, want %q", abstention, legalAbstentionMultipleEntities)
	}
}

// The abstention counts entities. Two rows that survived dedupe under one
// name are the case it is for, and counting names answered "one company" to
// exactly that census.
func TestTheAbstentionCountsEntitiesRatherThanNames(t *testing.T) {
	sameName := []corpusLegalEntity{
		{Name: "Acme GmbH", RegisterNumber: "HRB 111", SourceURL: seedURL + "/impressum"},
		{Name: "Acme GmbH", RegisterNumber: "HRB 222", SourceURL: seedURL + "/impressum"},
	}
	if got := legalAbstentionOf(sameName, false); got != legalAbstentionMultipleEntities {
		t.Errorf("two registrations printed under one name gave %q, want %q", got, legalAbstentionMultipleEntities)
	}
}

// And the whole path, because the abstention is only worth having if the trio
// it guards is actually withheld: applyLegalGate must strip the legal fields
// on this census rather than publish one company's address as the other's.
func TestTheLegalTrioIsWithheldWhenOneNameCarriesTwoRegistrations(t *testing.T) {
	entities := dedupeLegalEntities([]corpusLegalEntity{
		{Name: "Acme GmbH", RegisteredAddress: "Weg 1 11111 Ort", SourceURL: seedURL + "/impressum"},
		{Name: "Acme GmbH", RegisterNumber: "HRB 111", SourceURL: seedURL + "/de/impressum"},
		{Name: "Acme GmbH", RegisterNumber: "HRB 222", SourceURL: seedURL + "/en/imprint"},
	})
	fields := []evidencedField{
		{Field: fieldRegisteredAddress, Value: "Weg 1 11111 Ort", SourceURL: seedURL + "/impressum"},
	}
	kinds := map[string]crmcontracts.SiteReadPageKind{
		seedURL + "/impressum": crmcontracts.SiteReadPageKindImpressum,
	}
	kept, abstained, dropped := applyLegalGate(fields, entities, kinds, false)
	if !abstained {
		t.Fatalf("the gate answered a contested census without abstaining: %+v", entities)
	}
	if len(kept) != 0 {
		t.Errorf("a legal field survived a contested census: %+v", kept)
	}
	if len(dropped) == 0 || dropped[0].Reason != string(legalAbstentionMultipleEntities) {
		t.Errorf("the withheld field must name the cause that fired: %+v", dropped)
	}
}

// The guard must not split a company that simply appears twice. Over-
// correction here returns a census of duplicates and abstains on every
// multi-locale imprint, which is the shape of guard that gets turned off.
func TestARepeatedSightingWithNoIdentifierStillFolds(t *testing.T) {
	entities := []corpusLegalEntity{
		{Name: "Acme GmbH", RegisterNumber: "HRB 111", SourceURL: seedURL + "/impressum"},
		{Name: "Acme GmbH", RegisterNumber: "HRB 222", SourceURL: seedURL + "/en/imprint"},
		// A market heading repeated under both blocks, printing no identity.
		{Name: "Acme Deutschland", SourceURL: seedURL + "/impressum"},
		{Name: "Acme Deutschland", SourceURL: seedURL + "/de/impressum"},
	}
	got := dedupeLegalEntities(entities)
	headings := 0
	for _, entity := range got {
		if legalEntityNameKey(entity.Name) == legalEntityNameKey("Acme Deutschland") {
			headings++
		}
	}
	if headings != 1 {
		t.Errorf("a repeated heading became %d entities; sightings carrying nothing to tell apart fold: %+v",
			headings, got)
	}
}
