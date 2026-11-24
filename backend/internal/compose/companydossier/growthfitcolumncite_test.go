// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package companydossier

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/claims"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func columnIndustryReply(in Input, entityType, entityID string) string {
	return columnReply(in, "They work in healthcare.", entityType, entityID)
}

func columnReply(in Input, text, entityType, entityID string) string {
	return cited(in, `{"band":"strong","sub_scores":[],"positive_factors":[{"text":"`+text+`",`+
		`"nature":"fact","evidence":[{"entity_type":"`+entityType+`","entity_id":"`+entityID+`"}]}],`+
		`"negative_factors":[],"whitespace":[],"objections":[],"recommended_angle":null}`)
}

func TestAnIndustryOnlyTheColumnHoldsIsCitable(t *testing.T) {
	for name, tc := range map[string]struct {
		in    Input
		claim string
	}{
		"industry":  {Input{CompanyID: ids.NewV7().String(), Industry: "healthcare"}, "They work in healthcare."},
		"size band": {Input{CompanyID: ids.NewV7().String(), SizeBand: "51-200"}, "They have 51-200 employees."},
	} {
		in := tc.in
		t.Run(name, func(t *testing.T) {
			_, kept, err := ParseGrowthFit(columnReply(in, tc.claim, citeCompany, in.CompanyID), in)
			if err != nil {
				t.Fatal(err)
			}
			if len(kept.PositiveFactors) != 1 || kept.PositiveFactors[0].Evidence[0].EntityID != in.CompanyID {
				t.Fatalf("a sentence citing the company column was dropped: %+v", kept.PositiveFactors)
			}
			for _, request := range []string{GrowthFitRequest(in, "en").Messages[0].Content, DossierRequest(in, "en").Messages[0].Content} {
				if claims.SpellsRecordID(request) {
					t.Errorf("the request spells the company id instead of its ref:\n%s", request)
				}
			}
		})
	}
}

func TestACompanyWithNeitherColumnIsNotCitable(t *testing.T) {
	in := fourOfSeven()
	if _, held := KnownRecords(in)[claims.Evidence{EntityType: citeCompany, EntityID: in.CompanyID}]; held {
		t.Fatal("a company with no industry or size band grounds sentences on its id alone")
	}
	// The parse refuses a reply that grounds nothing, which is the drop too.
	_, kept, err := ParseGrowthFit(columnIndustryReply(in, citeCompany, in.CompanyID), in)
	if err == nil && len(kept.PositiveFactors) != 0 {
		t.Errorf("a citation of a company with no columns was kept: %+v", kept.PositiveFactors)
	}
}

func TestTheColumnReplacesTheSiteReadValueItContradicts(t *testing.T) {
	stale := assessedAt.Add(-400 * 24 * time.Hour)
	oldIndustry := machineField("industry", "Retail", stale)
	oldRange := machineFact("employee_range", "1-10", stale)
	offer := machineField("offer_summary", "Hospital software", stale)
	tech := machineFact("technology", "SAP", stale)
	facts := stubFacts{in: Input{
		Industry: "healthcare", SizeBand: "51-200",
		ProfileFields: []crmcontracts.CompanyProfileField{oldIndustry, offer},
		Facts:         []crmcontracts.CompanyFact{oldRange, tech},
	}}
	companyID := ids.New[ids.CompanyKind]()
	in, err := BuildInput(context.Background(), facts, companyID)
	if err != nil {
		t.Fatal(err)
	}
	known := KnownRecords(in)
	for _, gone := range []claims.Evidence{
		{EntityType: citeProfileField, EntityID: oldIndustry.Id.String()},
		{EntityType: citeFact, EntityID: oldRange.Id.String()},
	} {
		if _, held := known[gone]; held {
			t.Errorf("%s %s contradicts the column and is still citable", gone.EntityType, gone.EntityID)
		}
	}
	for _, kept := range []claims.Evidence{
		{EntityType: citeProfileField, EntityID: offer.Id.String()},
		{EntityType: citeFact, EntityID: tech.Id.String()},
		{EntityType: citeCompany, EntityID: in.CompanyID},
	} {
		if _, held := known[kept]; !held {
			t.Errorf("%s %s was dropped, but nothing replaces it", kept.EntityType, kept.EntityID)
		}
	}
	if _, err := EvidenceFor(context.Background(), facts, companyID,
		citeProfileField, *oldIndustry.Id, "en"); err == nil {
		t.Error("the receipt of the replaced industry field still opens, so it can contradict the fit")
	}
}

func TestWithoutTheColumnTheSiteReadValueStays(t *testing.T) {
	industry := machineField("industry", "Retail", assessedAt)
	size := machineFact("employee_range", "1-10", assessedAt)
	in, err := BuildInput(context.Background(), stubFacts{in: Input{
		ProfileFields: []crmcontracts.CompanyProfileField{industry},
		Facts:         []crmcontracts.CompanyFact{size},
	}}, ids.New[ids.CompanyKind]())
	if err != nil {
		t.Fatal(err)
	}
	if len(in.ProfileFields) != 1 || len(in.Facts) != 1 {
		t.Errorf("a company with no column lost its site-read industry or size: %+v %+v", in.ProfileFields, in.Facts)
	}
}

func TestTheFloorDossierStatesTheColumnIndustry(t *testing.T) {
	in := Input{CompanyID: ids.NewV7().String(), Industry: "healthcare"}
	sections := keepGrounded(Deterministic(in, "de"), in)
	for _, section := range sections {
		for _, sentence := range section.Sentences {
			if strings.Contains(sentence.Text, "Gesundheitswesen") && sentence.Evidence[0].EntityType == citeCompany {
				return
			}
		}
	}
	t.Errorf("the floor dossier does not state the column industry, cited to the company: %+v", sections)
}
