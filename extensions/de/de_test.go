// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package de

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/pkg/extension/jurisdiction"
	"github.com/margince/margince/backend/pkg/extension/messaging"
)

// TestNewDeclaresTheGoBDFloors pins the statutory content: the §147
// AO/HGB retention floors as CALENDAR years. A changed span or class
// name here is a legal-content change and must be deliberate.
func TestNewDeclaresTheGoBDFloors(t *testing.T) {
	e := New()
	if e.Name != "de" {
		t.Fatalf("Name = %q, want the unit's directory name de", e.Name)
	}
	if len(e.Jurisdictions) != 1 {
		t.Fatalf("declaration carries %d jurisdiction packs, want 1", len(e.Jurisdictions))
	}
	p := e.Jurisdictions[0]
	if got := p.Code(); got != "de" {
		t.Fatalf("pack code = %q, want de", got)
	}
	want := map[jurisdiction.RetentionClassName]jurisdiction.Period{
		jurisdiction.CommercialCorrespondence: {Years: 6},
		jurisdiction.AccountingRecords:        {Years: 8},
	}
	classes := p.Retention().Classes()
	if len(classes) != len(want) {
		t.Fatalf("pack declares %d retention classes, want %d", len(classes), len(want))
	}
	for _, c := range classes {
		keep, known := want[c.Name]
		if !known {
			t.Errorf("unexpected retention class %q", c.Name)
			continue
		}
		if c.Keep != keep {
			t.Errorf("class %s keeps %s, want %s (statutory floor, calendar years)", c.Name, c.Keep, keep)
		}
	}
}

// The German messaging matrix, asserted rather than described.
//
// This is the rule set a reviewer ratifies, so each claim it makes is a test:
// what §7(3) requires before advertising without consent, what Art. 13 puts on
// a first message, and — as load-bearing as either — the three obligations
// German law does NOT impose, which a reader comparing packs must be able to
// tell apart from an omission.

// UWG §7(3) permits email advertising to an existing customer only where all
// four conditions hold. The pack declares all four; declaring three would be an
// exception the engine applies while checking less than the statute asks, which
// is worse than none because it looks lawful.
func TestTheExistingCustomerExceptionCarriesAllFourConditions(t *testing.T) {
	rules := messagingRules()
	if len(rules.MarketingExceptions) != 1 {
		t.Fatalf("%d marketing exceptions declared, want exactly the §7(3) one", len(rules.MarketingExceptions))
	}
	e := rules.MarketingExceptions[0]
	if e.Kind != messaging.ExistingCustomer {
		t.Fatalf("exception kind = %q, want existing_customer", e.Kind)
	}
	for _, c := range []struct {
		name string
		set  bool
	}{
		{"sale evidence (address obtained in connection with a sale)", e.RequiresSaleEvidence},
		{"collection-time opt-out notice", e.RequiresCollectionTimeOptOut},
		{"similarity to what was bought", e.RequiresSimilarity},
		{"no standing objection", e.RequiresNoObjection},
	} {
		if !c.set {
			t.Errorf("§7(3) requires %s and the pack does not ask the engine to check it", c.name)
		}
	}
}

// Art. 13 at first contact: who is writing, who answers about the data, and how
// to say stop. The objection route is marketing-scoped because §7(3) requires it
// at every use rather than only the first.
func TestAFirstMessageDisclosesTheController(t *testing.T) {
	want := map[messaging.DisclosureKind]bool{
		messaging.ControllerIdentity: false,
		messaging.PrivacyContact:     false,
		messaging.ObjectionRoute:     true,
	}
	got := map[messaging.DisclosureKind]bool{}
	for _, d := range messagingRules().Disclosures {
		got[d.Kind] = d.MarketingOnly
	}
	for kind, marketingOnly := range want {
		scope, declared := got[kind]
		if !declared {
			t.Errorf("a first message does not disclose %q", kind)
			continue
		}
		if scope != marketingOnly {
			t.Errorf("%q is marketing-only=%v, want %v", kind, scope, marketingOnly)
		}
	}
}

// German law imposes no subject prefix on commercial email, no statutory
// frequency ceiling, and owes no acknowledgement for an opt-out. The zero
// values say so, and this test is what makes the silence deliberate: a reader
// comparing this pack against one that DOES impose them can tell a considered
// absence from a forgotten field.
func TestGermanyImposesNoPrefixNoCapAndNoAcknowledgement(t *testing.T) {
	rules := messagingRules()
	if rules.SubjectPrefix != "" {
		t.Errorf("the pack prefixes advertising subjects with %q; German law requires no marking", rules.SubjectPrefix)
	}
	if rules.FrequencyCap != nil {
		t.Errorf("the pack caps advertising at %+v; German law sets no statutory ceiling", rules.FrequencyCap)
	}
	if rules.OptOutAcknowledgement {
		t.Error("the pack promises an opt-out acknowledgement; German law owes none")
	}
}

// A reply stays a reply for a year and a deal follow-up for six months. Neither
// bounds a same-thread reply — the subject wrote to us and did not withdraw —
// so these windows only reach an UNPROMPTED follow-up. A pack that shortened
// them would be refusing correspondence rather than restricting advertising.
func TestTheWindowsBoundAnUnpromptedFollowUp(t *testing.T) {
	rules := messagingRules()
	if rules.ReplyWindow != 365*24*time.Hour {
		t.Errorf("reply window = %s, want twelve months", rules.ReplyWindow)
	}
	if rules.DealFollowUpWindow != 182*24*time.Hour {
		t.Errorf("deal follow-up window = %s, want six months", rules.DealFollowUpWindow)
	}
	if rules.DealFollowUpWindow >= rules.ReplyWindow {
		t.Error("a deal follow-up outlasts a reply, which inverts which evidence is stronger")
	}
}

// The declared rules pass the published validator, which is what the boot
// preflight runs. A pack that composed and then failed at boot would be found
// by an operator rather than by this suite.
func TestTheDeclaredRulesPassThePublishedValidator(t *testing.T) {
	if err := messagingRules().Validate(); err != nil {
		t.Errorf("the German rules would be refused at boot: %v", err)
	}
}

// The unit declares its rules under its own jurisdiction, so the registry keys
// them where the engine looks.
func TestTheUnitDeclaresItsMessagingRules(t *testing.T) {
	declared := New().Messaging
	if len(declared) != 1 {
		t.Fatalf("%d messaging rule sets declared, want 1", len(declared))
	}
	if declared[0].Jurisdiction != New().Jurisdictions[0].Code() {
		t.Errorf("messaging rules name jurisdiction %q while the pack is %q",
			declared[0].Jurisdiction, New().Jurisdictions[0].Code())
	}
}
