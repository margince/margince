// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package eu

import (
	"slices"
	"testing"

	"github.com/margince/margince/backend/pkg/extension/jurisdiction"
	"github.com/margince/margince/backend/pkg/extension/messaging"
)

// memberStates are the 27 EU member states, as lower-case ISO 3166-1 codes.
var memberStates = []jurisdiction.Code{
	"at", "be", "bg", "cy", "cz", "de", "dk", "ee", "es", "fi", "fr", "gr", "hr", "hu",
	"ie", "it", "lt", "lu", "lv", "mt", "nl", "pl", "pt", "ro", "se", "si", "sk",
}

func TestEveryMemberStateRegistersOnePackAndOneRuleSet(t *testing.T) {
	e := New()
	if e.Name != "eu" {
		t.Fatalf("Name = %q, want the unit's directory name eu", e.Name)
	}
	var packs, rules []jurisdiction.Code
	for _, p := range e.Jurisdictions {
		packs = append(packs, p.Code())
	}
	for _, r := range e.Messaging {
		rules = append(rules, r.Jurisdiction)
		if err := r.Validate(); err != nil {
			t.Errorf("%s: the rules would be refused at boot: %v", r.Jurisdiction, err)
		}
	}
	slices.Sort(packs)
	slices.Sort(rules)
	if !slices.Equal(packs, memberStates) {
		t.Errorf("jurisdiction packs %v, want one per member state %v", packs, memberStates)
	}
	if !slices.Equal(rules, memberStates) {
		t.Errorf("messaging rule sets %v, want one per member state %v", rules, memberStates)
	}
}

// Germany alone carries retention floors. Every other member state's duties
// bind books and invoices, which a CRM does not hold.
func TestOnlyGermanyDeclaresRetentionFloors(t *testing.T) {
	for _, p := range New().Jurisdictions {
		classes := p.Retention().Classes()
		if p.Code() == "de" && len(classes) == 0 {
			t.Error("the German pack lost its GoBD floors")
		}
		if p.Code() != "de" && len(classes) != 0 {
			t.Errorf("%s declares %d retention classes, want none", p.Code(), len(classes))
		}
	}
}

// A pack offers no exception where national law adds a condition the engine
// cannot check (Austria's ECG list, Ireland's 12 months). It offers none where
// the law names no exception (Bulgaria, Hungary, Poland). Every other member
// state keeps the four conditions of Art. 13(2).
func TestTheExistingCustomerExceptionFollowsNationalLaw(t *testing.T) {
	without := []jurisdiction.Code{"at", "bg", "hu", "ie", "pl"}
	for _, r := range New().Messaging {
		if slices.Contains(without, r.Jurisdiction) {
			if len(r.MarketingExceptions) != 0 {
				t.Errorf("%s offers %+v, want no exception", r.Jurisdiction, r.MarketingExceptions)
			}
			continue
		}
		want := []messaging.MarketingException{{
			Kind: messaging.ExistingCustomer, RequiresSaleEvidence: true, RequiresCollectionTimeOptOut: true,
			RequiresSimilarity: true, RequiresNoObjection: true,
		}}
		if !slices.Equal(r.MarketingExceptions, want) {
			t.Errorf("%s offers %+v, want the four conditions of Art. 13(2)", r.Jurisdiction, r.MarketingExceptions)
		}
	}
}

// The national duties beyond the EU layer that the engine can apply.
func TestNationalAdditionsReachTheirPack(t *testing.T) {
	for _, r := range New().Messaging {
		if got, want := r.SubjectPrefix, map[jurisdiction.Code]string{"es": "Publicidad"}[r.Jurisdiction]; got != want {
			t.Errorf("%s: subject prefix %q, want %q (LSSI art. 20 in Spain, none elsewhere)", r.Jurisdiction, got, want)
		}
		if got := r.ConsentConfirmationNeedsRequest; got != (r.Jurisdiction == "it") {
			t.Errorf("%s: a consent request needs the recipient's own request = %v, want it in Italy alone", r.Jurisdiction, got)
		}
	}
}

// Every pack is a draft until a legal review stamps it, so none may show a
// citation to a rep.
func TestEveryPackIsAnUnreviewedDraft(t *testing.T) {
	for _, r := range New().Messaging {
		if !r.Provenance.ReviewedAt.IsZero() || r.Provenance.Reviewer != "" || len(r.Citations) != 0 {
			t.Errorf("%s carries review %+v and %d citations, want a draft with none", r.Jurisdiction, r.Provenance, len(r.Citations))
		}
	}
}
