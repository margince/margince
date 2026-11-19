// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package vn

import (
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/margince/margince/backend/pkg/extension/messaging"
	"github.com/margince/margince/backend/pkg/extension/subjectrights"
)

// TestNewDeclaresTheJurisdictionAndNoRetentionClass pins the two structural
// facts: this pack is Vietnam, and it declares no retention floor. The absent
// class is deliberate content, so an added one must be an argued change rather
// than a copy of extensions/de.
func TestNewDeclaresTheJurisdictionAndNoRetentionClass(t *testing.T) {
	e := New()
	if e.Name != "vn" {
		t.Fatalf("Name = %q, want the unit's directory name vn", e.Name)
	}
	if len(e.Jurisdictions) != 1 {
		t.Fatalf("declaration carries %d jurisdiction packs, want 1", len(e.Jurisdictions))
	}
	if got := e.Jurisdictions[0].Code(); got != "vn" {
		t.Fatalf("pack code = %q, want vn", got)
	}
	if classes := e.Jurisdictions[0].Retention().Classes(); len(classes) != 0 {
		t.Fatalf("pack declares %d retention classes, want none — a CRM holds no record the Vietnamese floors bind", len(classes))
	}
}

// TestTheRulesAreValid runs the same check the boot preflight runs, so a rule
// set this pack could not be composed with fails here rather than at startup.
func TestTheRulesAreValid(t *testing.T) {
	if err := messagingRules().Validate(); err != nil {
		t.Fatalf("the declared rules do not validate: %v", err)
	}
}

// TestAdvertisingNeedsPriorConsent is the legal heart of the pack: Decree
// 91/2020/ND-CP Art. 10 grants NO route to advertising email without the
// recipient's prior consent. An added exception here would let a sale — or
// German existing-customer evidence, which the core fold could otherwise carry
// across — authorize a Vietnamese advertising message.
func TestAdvertisingNeedsPriorConsent(t *testing.T) {
	if got := messagingRules().MarketingExceptions; len(got) != 0 {
		t.Fatalf("the pack declares %d marketing exceptions, want none — prior consent is the only route", len(got))
	}
}

// TestTheAdvertisingLabelIsTheDecreesOwn pins the subject marking. The literal
// is legal content: a changed label stops the message being labelled as the
// decree requires, and nothing else in the tree would notice.
func TestTheAdvertisingLabelIsTheDecreesOwn(t *testing.T) {
	if got := messagingRules().SubjectPrefix; got != "[QC]" {
		t.Fatalf("SubjectPrefix = %q, want the Art. 18 advertising label [QC]", got)
	}
}

// TestTheDailyCeilingIsThreePerAddress pins Art. 13.5.
func TestTheDailyCeilingIsThreePerAddress(t *testing.T) {
	ceiling := messagingRules().FrequencyCap
	if ceiling == nil {
		t.Fatal("the pack declares no frequency cap, want three advertising messages per 24 hours")
	}
	if ceiling.Messages != 3 {
		t.Errorf("cap allows %d messages, want 3", ceiling.Messages)
	}
	if ceiling.Window != 24*time.Hour {
		t.Errorf("cap window = %s, want 24h", ceiling.Window)
	}
}

// TestAnOptOutIsAcknowledged pins Art. 20.
func TestAnOptOutIsAcknowledged(t *testing.T) {
	if !messagingRules().OptOutAcknowledgement {
		t.Fatal("the pack owes no opt-out acknowledgement, want one — Art. 20 requires a receipt at once")
	}
	if within := messagingRules().OptOutAcknowledgementWithin; within != time.Hour {
		t.Errorf("an unsent receipt raises its alarm after %s, want 1h: Art. 20.3 owes it at once", within)
	}
	if keep := messagingRules().OptOutRecordsKeep; keep < 366*24*time.Hour {
		t.Errorf("opt-out records are kept %s, want at least a year in every year: Art. 11.7", keep)
	}
	if offset := messagingRules().OptOutAcknowledgementUTCOffset; offset != 7*time.Hour {
		t.Errorf("the receipt prints its times at UTC%+v, want UTC+7: Art. 20.4(a) times are read in Vietnam", offset)
	}
}

// TestAnAdvertisingMessageNamesItsAdvertiser holds Art. 19 alongside the
// controller disclosures. Advertiser contact binds ADVERTISING only: an
// operational message carries who is processing the data, not who is selling.
func TestAnAdvertisingMessageNamesItsAdvertiser(t *testing.T) {
	want := map[messaging.DisclosureKind]bool{
		messaging.ControllerIdentity: false,
		messaging.PrivacyContact:     false,
		messaging.ObjectionRoute:     true,
		messaging.AdvertiserContact:  true,
	}
	got := map[messaging.DisclosureKind]bool{}
	for _, d := range messagingRules().Disclosures {
		if _, seen := got[d.Kind]; seen {
			t.Fatalf("disclosure %q declared twice", string(d.Kind))
		}
		got[d.Kind] = d.MarketingOnly
	}
	if len(got) != len(want) {
		t.Fatalf("the pack declares %d disclosures, want %d", len(got), len(want))
	}
	for kind, marketingOnly := range want {
		bound, declared := got[kind]
		if !declared {
			t.Errorf("disclosure %q is not declared", string(kind))
			continue
		}
		if bound != marketingOnly {
			t.Errorf("disclosure %q binds marketing-only = %v, want %v", string(kind), bound, marketingOnly)
		}
	}
}

// TestThePackStatesTheInstrumentsItsRulesRestOn.
//
// A decision records "vn version 2", which is opaque on its own: it answers a
// subject's question only for somebody holding this source tree at that moment.
// The instruments say what the number meant, in the words a regulator uses.
//
// THE 2020 DECREE IS STILL ONE OF THEM. Law 91/2025 and Decree 356/2025 amended
// the regime around it rather than replacing it, so a pack listing only the
// 2025 pair would misdescribe where the daily ceiling and the [QC] label come
// from — both are still the decree's.
func TestThePackStatesTheInstrumentsItsRulesRestOn(t *testing.T) {
	want := map[string]time.Time{
		"Decree 91/2020/ND-CP":  time.Date(2020, 10, 1, 0, 0, 0, 0, time.UTC),
		"Law 91/2025/QH15":      time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		"Decree 356/2025/ND-CP": time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	got := map[string]time.Time{}
	for _, in := range messagingRules().Instruments {
		if _, seen := got[in.Name]; seen {
			t.Fatalf("instrument %q declared twice", in.Name)
		}
		got[in.Name] = in.EffectiveFrom
	}
	if len(got) != len(want) {
		t.Fatalf("the pack states %d instruments, want %d: %v", len(got), len(want), got)
	}
	for name, from := range want {
		stated, declared := got[name]
		if !declared {
			t.Errorf("instrument %q is not stated — a decision recording this version "+
				"cannot be read back against it", name)
			continue
		}
		if !stated.Equal(from) {
			t.Errorf("instrument %q takes effect %s, want its own commencement date %s — "+
				"never the date this product learned about it",
				name, stated.Format(time.DateOnly), from.Format(time.DateOnly))
		}
	}
}

// TestTheVersionMovedWithTheInstruments. The number exists to tell two records
// apart, so it is meaningless if it stays put when the law it names changes.
func TestTheVersionMovedWithTheInstruments(t *testing.T) {
	if got := messagingRules().Version; got != 5 {
		t.Errorf("ruleset version = %d, want 5 — the refusal means, the refusal "+
			"section and the claims needing proof changed what a decision was judged under", got)
	}
}

func TestDataProtectionFollowsAVietnameseRecipient(t *testing.T) {
	rules := messagingRules()
	if !rules.DataProtectionFollowsRecipient {
		t.Error("the pack does not follow the recipient; Law 91/2025 Art. 1.2(c) reaches a foreign controller")
	}
	if rules.DisclosureLanguage != "vi" {
		t.Errorf("disclosure language = %q, want vi", rules.DisclosureLanguage)
	}
	if !rules.NoLegitimateInterests {
		t.Error("the pack allows legitimate interests; Vietnamese law has no such basis")
	}
}

func TestRequestsAreCarriedOutWithinTheDecreesDays(t *testing.T) {
	rules := subjectRights()
	if err := rules.Validate(); err != nil {
		t.Fatalf("the declared deadlines do not validate: %v", err)
	}
	for kind, days := range map[subjectrights.RequestKind]int{
		subjectrights.Rectify: 10, subjectrights.Erasure: 20, subjectrights.Restriction: 15,
	} {
		if got := rules.Deadlines[kind].Period.Days; got != days {
			t.Errorf("%s is due in %d days, want %d", kind, got, days)
		}
	}
	if rules.UTCOffset != 7*time.Hour {
		t.Errorf("working days are counted at offset %s, want Vietnam time, UTC+07:00", rules.UTCOffset)
	}
	if rules.ReplyWithinWorkingDays != 2 {
		t.Errorf("a first answer is owed in %d working days, want 2", rules.ReplyWithinWorkingDays)
	}
	if _, declared := rules.Deadlines[subjectrights.Access]; declared {
		t.Error("the pack declares an access deadline the decree does not set")
	}
}

// TestRefusalIsPossibleByWebEmailAndPhone pins Decree 91/2020/ND-CP Art. 20.2.
func TestRefusalIsPossibleByWebEmailAndPhone(t *testing.T) {
	for _, d := range messagingRules().Disclosures {
		if d.Kind != messaging.ObjectionRoute {
			continue
		}
		want := []messaging.ContactMeans{messaging.MeansWeb, messaging.MeansEmail, messaging.MeansPhone}
		if !slices.Equal(d.Means, want) {
			t.Errorf("the objection route asks for %v, want %v", d.Means, want)
		}
		return
	}
	t.Fatal("the pack declares no objection route")
}

// TestAdvertisingIsVietnameseFirstAndEndsWithTheRefusal pins the Law on
// Advertising Art. 18 and Decree 91/2020/ND-CP Art. 19 and 20.1.
func TestAdvertisingIsVietnameseFirstAndEndsWithTheRefusal(t *testing.T) {
	r := messagingRules()
	if r.DisclosureLanguage != "vi" {
		t.Errorf("disclosure language = %q, want vi: advertising leads with it", r.DisclosureLanguage)
	}
	if !r.RefusalSectionLast {
		t.Error("advertising does not end with the refusal section")
	}
}

// TestSuperlativesNeedAProof pins Law on Advertising Art. 8.11 and Circular
// 12/2026/TT-BVHTTDL Art. 3.1, which reaches a claim in any language.
func TestSuperlativesNeedAProof(t *testing.T) {
	want := []string{
		"nhất", "duy nhất", "tốt nhất", "số một", "số 1", "hàng đầu",
		"best", "no. 1", "number one", "leading", "the only", "unique",
	}
	if got := messagingRules().ClaimsNeedProof; !slices.Equal(got, want) {
		t.Errorf("the claims that need a proof are %q, want %q", got, want)
	}
}

// TestAForeignAdvertiserIsToldToUseAVietnameseAgency pins Law on Advertising
// Art. 39.2.
func TestAForeignAdvertiserIsToldToUseAVietnameseAgency(t *testing.T) {
	if !messagingRules().ForeignAdvertiserNeedsLocalAgency {
		t.Error("an advertiser outside Vietnam is not told to advertise through a Vietnamese agency")
	}
}

// TestOnlyAdvertisingConsentNeedsTheRequestFirst pins Decree 91/2020/ND-CP
// Art. 11.2, which governs advertising by mail and by phone: a consent request
// for correspondence needs no request of the contact's own.
func TestOnlyAdvertisingConsentNeedsTheRequestFirst(t *testing.T) {
	r := messagingRules()
	want := []messaging.PurposeClass{messaging.PurposeClassMarketing, messaging.PurposeClassPhoneOutreach}
	if !r.ConsentConfirmationNeedsRequest || !slices.Equal(r.ConsentRequestFirstFor, want) {
		t.Errorf("the request rule covers %v, want the advertising classes %v", r.ConsentRequestFirstFor, want)
	}
}

// TestTheCompiledRulesAreTheV5Rules pins every value of the compiled rule set.
// Provenance and the digest describe the policy source, so they are left out.
func TestTheCompiledRulesAreTheV5Rules(t *testing.T) {
	commencement2025 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	want := messaging.Rules{
		Jurisdiction: "vn",
		Version:      5,
		Instruments: []messaging.Instrument{
			{Name: "Decree 91/2020/ND-CP", EffectiveFrom: time.Date(2020, 10, 1, 0, 0, 0, 0, time.UTC)},
			{Name: "Law 91/2025/QH15", EffectiveFrom: commencement2025},
			{Name: "Decree 356/2025/ND-CP", EffectiveFrom: commencement2025},
		},
		ReplyWindow:        365 * 24 * time.Hour,
		DealFollowUpWindow: 182 * 24 * time.Hour,
		ConsentOnly:        true,
		Disclosures: []messaging.Disclosure{
			{Kind: messaging.ControllerIdentity},
			{Kind: messaging.PrivacyContact},
			{Kind: messaging.ObjectionRoute, MarketingOnly: true,
				Means: []messaging.ContactMeans{messaging.MeansWeb, messaging.MeansEmail, messaging.MeansPhone}},
			{Kind: messaging.AdvertiserContact, MarketingOnly: true},
		},
		RefusalSectionLast: true,
		ClaimsNeedProof: []string{
			"nhất", "duy nhất", "tốt nhất", "số một", "số 1", "hàng đầu",
			"best", "no. 1", "number one", "leading", "the only", "unique",
		},
		ForeignAdvertiserNeedsLocalAgency: true,
		SubjectPrefix:                     "[QC]",
		FrequencyCap:                      &messaging.FrequencyCap{Messages: 3, Window: 24 * time.Hour},
		OptOutAcknowledgement:             true,
		OptOutAcknowledgementWithin:       time.Hour,
		OptOutAcknowledgementUTCOffset:    7 * time.Hour,
		OptOutRecordsKeep:                 366 * 24 * time.Hour,
		DataProtectionFollowsRecipient:    true,
		DisclosureLanguage:                "vi",
		NoLegitimateInterests:             true,
		ConsentConfirmationNeedsRequest:   true,
		ConsentRequestFirstFor:            messaging.AdvertisingPurposeClasses(),
	}
	got := messagingRules()
	got.Provenance, got.Digest = messaging.Provenance{}, ""
	if !reflect.DeepEqual(got, want) {
		t.Errorf("the compiled rules differ from the v5 rules:\n got %+v\nwant %+v", got, want)
	}
}
