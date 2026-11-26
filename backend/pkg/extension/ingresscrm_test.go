// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package extension_test

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/pkg/extension"
)

func aCompanyRecord() extension.Record {
	return extension.Record{
		System: "crm-x", Key: "acct-1",
		Company: &extension.CompanyFields{DisplayName: "Acme", Domain: "acme.test", AdoptOn: extension.AdoptKeyDomain},
	}
}

func aDealRecord() extension.Record {
	return extension.Record{
		System: "crm-x", Key: "opp-1",
		Deal: &extension.DealFields{Name: "Acme renewal", PipelineID: "0191a000-0000-7000-8000-000000000001", StageID: "0191a000-0000-7000-8000-000000000002"},
	}
}

func TestAnIngestRecordNamesTheKindItsPayloadCarries(t *testing.T) {
	for name, tc := range map[string]struct {
		rec  extension.Record
		want extension.RecordKind
	}{
		"activity": {aValidRecord(), extension.KindActivity},
		"company":  {aCompanyRecord(), extension.KindCompany},
		"deal":     {aDealRecord(), extension.KindDeal},
	} {
		if got := tc.rec.Kind(); got != tc.want {
			t.Errorf("%s: Kind() = %q, want %q", name, got, tc.want)
		}
		if err := tc.rec.Validate(); err != nil {
			t.Errorf("%s: a well-formed record was refused: %v", name, err)
		}
	}
}

func TestARecordWhosePayloadDisagreesWithItselfIsRefused(t *testing.T) {
	both := aCompanyRecord()
	both.Deal = aDealRecord().Deal
	withActivity := aCompanyRecord()
	withActivity.Activity = aValidRecord().Activity
	for name, rec := range map[string]extension.Record{
		"two payloads":                   both,
		"a company carrying an activity": withActivity,
	} {
		if err := rec.Validate(); extension.RefusalOf(err) != extension.RefusalActivity {
			t.Errorf("%s: err = %v, want a %q refusal", name, err, extension.RefusalActivity)
		}
	}
}

func TestACompanyOrDealPayloadIsHeldToItsGrammar(t *testing.T) {
	for name, edit := range map[string]func(*extension.Record){
		"a company with no name":      func(r *extension.Record) { r.Company.DisplayName = "" },
		"an adopt key the core lacks": func(r *extension.Record) { r.Company.AdoptOn = "vat" },
		"adopting on a blank domain":  func(r *extension.Record) { r.Company.Domain = "" },
		"a company name over the cap": func(r *extension.Record) {
			r.Company.DisplayName = strings.Repeat("n", extension.MaxDisplayNameRunes+1)
		},
		"a relink key over the cap": func(r *extension.Record) {
			r.Company.RelinkActivities = []string{strings.Repeat("k", extension.MaxKeyLength+1)}
		},
	} {
		rec := aCompanyRecord()
		edit(&rec)
		if err := rec.Validate(); extension.RefusalOf(err) != extension.RefusalActivity {
			t.Errorf("%s: err = %v, want a payload refusal", name, err)
		}
	}
	for name, edit := range map[string]func(*extension.Record){
		"a deal with no name":         func(r *extension.Record) { r.Deal.Name = "" },
		"a deal with no stage":        func(r *extension.Record) { r.Deal.StageID = "" },
		"a company that is not an id": func(r *extension.Record) { r.Deal.CompanyID = "acme" },
		"a stage that is not an id": func(r *extension.Record) {
			r.Deal.StageID = "not-a-uuid"
			r.Deal.PipelineID = "also-not"
		},
	} {
		rec := aDealRecord()
		edit(&rec)
		if err := rec.Validate(); extension.RefusalOf(err) != extension.RefusalActivity {
			t.Errorf("%s: err = %v, want a payload refusal", name, err)
		}
	}
}

func TestADealsCustomFieldsAreHeldToTheCfKeyGrammarAndCap(t *testing.T) {
	ok := aDealRecord()
	ok.Deal.CustomFields = map[string]any{"cf_route": "NBO-FRA", "cf_services": []string{"fuel"}}
	if err := ok.Validate(); err != nil {
		t.Fatalf("well-formed custom fields were refused: %v", err)
	}
	tooMany := make(map[string]any, extension.MaxLandedCustomFields+1)
	for i := 0; i <= extension.MaxLandedCustomFields; i++ {
		tooMany["cf_k"+strings.Repeat("a", i%10)+string(rune('a'+i/10))] = 1
	}
	for name, fields := range map[string]map[string]any{
		"a core column name": {"name": "x"},
		"an upper-case key":  {"cf_Route": "x"},
		"a bare cf_":         {"cf_": "x"},
		"too many keys":      tooMany,
	} {
		rec := aDealRecord()
		rec.Deal.CustomFields = fields
		if err := rec.Validate(); extension.RefusalOf(err) != extension.RefusalActivity {
			t.Errorf("%s: err = %v, want a payload refusal", name, err)
		}
	}
}

func TestASourceMayDeclareCompanyAndDealKinds(t *testing.T) {
	src := extension.IngressSource{System: "crm-x", Lands: []extension.RecordKind{extension.KindActivity, extension.KindCompany, extension.KindDeal}}
	if err := src.Validate(); err != nil {
		t.Fatalf("a source declaring all three kinds was refused: %v", err)
	}
	src.Lands = []extension.RecordKind{"lead"}
	if err := src.Validate(); err == nil {
		t.Fatal("a source declaring an unpublished kind was accepted")
	}
}

func TestARecordOverASizeCapIsRefused(t *testing.T) {
	longDesc := aCompanyRecord()
	longDesc.Company.Description = strings.Repeat("d", extension.MaxSubjectRunes+1)
	longDeal := aDealRecord()
	longDeal.Deal.Name = strings.Repeat("n", extension.MaxDisplayNameRunes+1)
	manyRelinks := aDealRecord()
	manyRelinks.Deal.RelinkActivities = make([]string, extension.MaxRelinkActivities+1)
	for i := range manyRelinks.Deal.RelinkActivities {
		manyRelinks.Deal.RelinkActivities[i] = "k"
	}
	for name, rec := range map[string]extension.Record{
		"a company description": longDesc, "a deal name": longDeal, "too many relinks": manyRelinks,
	} {
		if err := rec.Validate(); extension.RefusalOf(err) != extension.RefusalActivity {
			t.Errorf("%s: err = %v, want a payload refusal", name, err)
		}
	}

	bigCompany := aCompanyRecord()
	bigCompany.Raw = make([]byte, extension.MaxRawBytes+1)
	bigActivity := aValidRecord()
	bigActivity.Raw = make([]byte, extension.MaxRawBytes+1)
	for name, rec := range map[string]extension.Record{"a company's raw": bigCompany, "an activity's raw": bigActivity} {
		if err := rec.Validate(); extension.RefusalOf(err) != extension.RefusalSize {
			t.Errorf("%s: err = %v, want a size refusal", name, err)
		}
	}
}
