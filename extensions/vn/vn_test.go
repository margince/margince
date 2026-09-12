// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package vn

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/pkg/extension/messaging"
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
		t.Fatalf("SubjectPrefix = %q, want the Art. 12 advertising label [QC]", got)
	}
}

// TestTheDailyCeilingIsThreePerAddress pins Art. 22(2).
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

// TestAnOptOutIsAcknowledged pins Art. 16.
func TestAnOptOutIsAcknowledged(t *testing.T) {
	if !messagingRules().OptOutAcknowledgement {
		t.Fatal("the pack owes no opt-out acknowledgement, want one — Art. 16 requires a confirmation within 24 hours")
	}
}

// TestAnAdvertisingMessageNamesItsAdvertiser holds Art. 13 alongside the
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
	if got := messagingRules().Version; got != 2 {
		t.Errorf("ruleset version = %d, want 2 — the pack now states the 2025 "+
			"instruments and a decision must be able to say which set judged it", got)
	}
}
