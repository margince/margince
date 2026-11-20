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

// Every member state but Slovenia carries an accounting floor that names its
// law, and only the states whose law names business letters carry a
// correspondence floor.
func TestEveryMemberStateDeclaresItsStatutoryFloors(t *testing.T) {
	keepsLetters := []jurisdiction.Code{"at", "de", "es", "fi", "it", "lu", "mt", "pt"}
	for _, p := range New().Jurisdictions {
		classes := p.Retention().Classes()
		if p.Code() == "si" {
			if len(classes) != 0 {
				t.Errorf("si declares %d retention classes; no statutory period was confirmed", len(classes))
			}
			continue
		}
		assertFloors(t, p.Code(), classes, slices.Contains(keepsLetters, p.Code()))
	}
}

func assertFloors(t *testing.T, code jurisdiction.Code, classes []jurisdiction.RetentionClass, keepsLetters bool) {
	t.Helper()
	var letters, books bool
	for _, c := range classes {
		if err := c.Name.Validate(); err != nil {
			t.Errorf("%s: %v", code, err)
		}
		if c.Law == "" {
			t.Errorf("%s: the %s floor names no law", code, c.Name)
		}
		letters = letters || c.Name == jurisdiction.CommercialCorrespondence
		books = books || c.Name == jurisdiction.AccountingRecords
	}
	if !books {
		t.Errorf("%s declares no accounting floor", code)
	}
	if letters != keepsLetters {
		t.Errorf("%s correspondence floor = %t, want %t", code, letters, keepsLetters)
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

// citedEverywhere are the codes every member state's pack names the law for:
// the marketing refusal, the missing reason for contact, and the objection.
var citedEverywhere = []string{
	"no_marketing_consent", "marketing_permission_required", "no_compatible_evidence",
	"marketing_objection", "objection_route", "public_opt_out_link",
}

// No pack waits for a legal review to name its law, so a draft cites.
func TestEveryDraftPackCitesItsLaw(t *testing.T) {
	for _, r := range New().Messaging {
		if !r.Provenance.ReviewedAt.IsZero() {
			t.Errorf("%s carries a review %+v; the packs are drafts refreshed at release", r.Jurisdiction, r.Provenance)
		}
		for _, code := range citedEverywhere {
			if !slices.ContainsFunc(r.Citations, func(c messaging.Citation) bool { return c.Code == code }) {
				t.Errorf("%s cites no law for %s", r.Jurisdiction, code)
			}
		}
	}
}
