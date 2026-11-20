// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package apac

import (
	"slices"
	"testing"
	"time"

	"github.com/margince/margince/backend/pkg/extension/compliancedocs"
	"github.com/margince/margince/backend/pkg/extension/jurisdiction"
	"github.com/margince/margince/backend/pkg/extension/messaging"
)

// countries are the fourteen countries of this unit, as lower-case ISO 3166-1
// codes.
var countries = []jurisdiction.Code{
	"au", "cn", "hk", "id", "in", "jp", "kr", "my", "nz", "ph", "sg", "th", "tw", "vn",
}

func TestEveryCountryRegistersOnePackAndOneRuleSet(t *testing.T) {
	e := New()
	if e.Name != "apac" {
		t.Fatalf("Name = %q, want the unit's directory name apac", e.Name)
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
	if !slices.Equal(packs, countries) {
		t.Errorf("jurisdiction packs %v, want one per country %v", packs, countries)
	}
	if !slices.Equal(rules, countries) {
		t.Errorf("messaging rule sets %v, want one per country %v", rules, countries)
	}
}

// No country here declares a retention floor: their record-keeping duties
// bind books and invoices, which a CRM does not hold.
func TestNoCountryDeclaresARetentionFloor(t *testing.T) {
	for _, p := range New().Jurisdictions {
		if classes := p.Retention().Classes(); len(classes) != 0 {
			t.Errorf("%s declares %d retention classes, want none", p.Code(), len(classes))
		}
	}
}

// Vietnam alone states request deadlines; every other country gets the core's.
func TestOnlyVietnamDeclaresRequestDeadlines(t *testing.T) {
	var codes []jurisdiction.Code
	for _, r := range New().SubjectRights {
		codes = append(codes, r.Jurisdiction)
	}
	if !slices.Equal(codes, []jurisdiction.Code{"vn"}) {
		t.Errorf("subject-rights rules for %v, want Vietnam's alone", codes)
	}
}

func TestVietnamShipsEmployeeNoticeAndReceipt(t *testing.T) {
	documents := New().Documents
	if err := compliancedocs.ValidateSet(documents); err != nil {
		t.Fatal(err)
	}
	want := map[string]struct {
		language string
		kind     compliancedocs.Kind
	}{
		"thong-bao-cho-nguoi-lao-dong": {"vi", compliancedocs.Template},
		"employee-notice":              {"en", compliancedocs.ReadingCopy},
		"xac-nhan-da-nhan-thong-bao":   {"vi", compliancedocs.Template},
	}
	for _, document := range documents {
		expected, ok := want[document.ID]
		if !ok || document.Jurisdiction != "vn" || document.Audience != compliancedocs.Employee ||
			document.Language != expected.language || document.Kind != expected.kind {
			t.Errorf("unexpected employee document %q (%s, %s, %s)", document.ID, document.Jurisdiction, document.Language, document.Kind)
		}
		delete(want, document.ID)
	}
	if len(want) != 0 {
		t.Errorf("missing employee documents: %v", want)
	}
}

// Inferred consent (Australia, New Zealand) and the transaction relationship
// (Japan) are the only routes to advertising without consent, and each carries
// all four conditions. Every other country offers none.
func TestOnlyInferredConsentAndTheTransactionRelationshipOpenAnException(t *testing.T) {
	with := []jurisdiction.Code{"au", "jp", "nz"}
	want := []messaging.MarketingException{{
		Kind: messaging.ExistingCustomer, RequiresSaleEvidence: true, RequiresCollectionTimeOptOut: true,
		RequiresSimilarity: true, RequiresNoObjection: true,
	}}
	for _, r := range New().Messaging {
		if !slices.Contains(with, r.Jurisdiction) {
			if len(r.MarketingExceptions) != 0 {
				t.Errorf("%s offers %+v, want no exception", r.Jurisdiction, r.MarketingExceptions)
			}
			continue
		}
		if !slices.Equal(r.MarketingExceptions, want) {
			t.Errorf("%s offers %+v, want the exception with all four conditions", r.Jurisdiction, r.MarketingExceptions)
		}
	}
}

// The subject labels and the strict consent the statutes name.
func TestNationalLabelsAndConsentReachTheirPack(t *testing.T) {
	labels := map[jurisdiction.Code]string{"cn": "广告", "kr": "(광고)", "sg": "<ADV>", "vn": "[QC]"}
	consentOnly := []jurisdiction.Code{"cn", "kr", "vn"}
	noLegitimateInterests := []jurisdiction.Code{"cn", "in", "vn"}
	for _, r := range New().Messaging {
		if got, want := r.SubjectPrefix, labels[r.Jurisdiction]; got != want {
			t.Errorf("%s: subject prefix %q, want %q", r.Jurisdiction, got, want)
		}
		if got, want := r.ConsentOnly, slices.Contains(consentOnly, r.Jurisdiction); got != want {
			t.Errorf("%s: consent-only = %v, want %v", r.Jurisdiction, got, want)
		}
		if got, want := r.NoLegitimateInterests, slices.Contains(noLegitimateInterests, r.Jurisdiction); got != want {
			t.Errorf("%s: no legitimate interests = %v, want %v", r.Jurisdiction, got, want)
		}
		if r.DataProtectionFollowsRecipient != (r.Jurisdiction == "vn") {
			t.Errorf("%s: data protection follows the recipient = %v, want it in Vietnam alone",
				r.Jurisdiction, r.DataProtectionFollowsRecipient)
		}
	}
}

// Every country asks for a way to refuse advertising.
func TestEveryCountryAsksForAnObjectionRoute(t *testing.T) {
	for _, r := range New().Messaging {
		if !slices.ContainsFunc(r.Disclosures, func(d messaging.Disclosure) bool {
			return d.Kind == messaging.ObjectionRoute && d.MarketingOnly
		}) {
			t.Errorf("%s declares no objection route on advertising", r.Jurisdiction)
		}
	}
}

// Korea owes a refusal its result within 14 days, in Korean time
// (art. 50(7)); Vietnam's receipt is pinned in vn_test.go.
func TestAKoreanRefusalIsAnsweredWithinFourteenDays(t *testing.T) {
	for _, r := range New().Messaging {
		if r.Jurisdiction != "kr" {
			if r.OptOutAcknowledgement && r.Jurisdiction != "vn" {
				t.Errorf("%s owes an opt-out receipt, want Korea and Vietnam alone", r.Jurisdiction)
			}
			continue
		}
		if !r.OptOutAcknowledgement || r.OptOutAcknowledgementWithin != 14*24*time.Hour ||
			r.OptOutAcknowledgementUTCOffset != 9*time.Hour {
			t.Errorf("kr receipt: owed %v, within %s, at UTC%+v; want owed, 336h, UTC+9",
				r.OptOutAcknowledgement, r.OptOutAcknowledgementWithin, r.OptOutAcknowledgementUTCOffset)
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
