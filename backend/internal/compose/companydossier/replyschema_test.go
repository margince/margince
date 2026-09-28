// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package companydossier

// Both lanes send the shape their parser reads, so a constrained decoder
// cannot write a reply the parser refuses whole — and the schema admits the
// replies the parser keeps, so enforcing it never costs a good answer.

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
	"github.com/margince/margince/backend/internal/shared/schema"
)

// fullGrowthFit is a reply naming every key growthFitClaims decodes.
func fullGrowthFit(in Input, angle string) string {
	cite := `[{"entity_type":"profile_field","entity_id":"` + in.ProfileFields[0].Id.String() + `"}]`
	claim := func(nature string) string {
		return `{"text":"They sell load-shifting software.","nature":"` + nature + `","evidence":` + cite + `}`
	}
	if angle == "" {
		angle = claim("recommendation")
	}
	return `{"band":"strong",
		"sub_scores":[{"dimension":"industry_fit","score":80,"reason":"They sell to utilities.","evidence":` + cite + `}],
		"positive_factors":[` + claim("fact") + `],"negative_factors":[` + claim("assessment") + `],
		"whitespace":[],"objections":[],"recommended_angle":` + angle + `}`
}

func TestTheGrowthFitRequestHoldsTheAngleToAClaim(t *testing.T) {
	in := sevenOfSeven()
	req := GrowthFitRequest(in, string(textlang.English))
	if len(req.ResponseSchema) == 0 {
		t.Fatal("the growth-fit request carries no response schema, so a decoder may write any shape")
	}
	good := fullGrowthFit(in, "")
	if _, _, err := ParseGrowthFit(good, in); err != nil {
		t.Fatalf("the parser refused the well-formed reply the schema is checked against: %v", err)
	}
	if err := schema.ValidateJSON(req.ResponseSchema, good); err != nil {
		t.Errorf("the schema refuses a reply the parser keeps: %v", err)
	}
	// The production failure: the angle written as bare prose.
	bare := fullGrowthFit(in, `"Lead with their demand-charge pain."`)
	if err := schema.ValidateJSON(req.ResponseSchema, bare); err == nil {
		t.Error("the schema admits recommended_angle as a string, which the parser refuses the whole reply over")
	}
	missingNature := strings.Replace(good, `"nature":"recommendation",`, "", 1)
	if err := schema.ValidateJSON(req.ResponseSchema, missingNature); err == nil {
		t.Error("the schema admits an angle with no nature, so its keys are not required")
	}
}

func TestTheDossierRequestSendsTheShapeItsParserReads(t *testing.T) {
	in := sevenOfSeven()
	req := DossierRequest(in, string(textlang.English))
	good := `{"sections":[{"kind":"summary","sentences":[{"text":"They sell load-shifting software.","nature":"fact",
		"evidence":[{"entity_type":"profile_field","entity_id":"` + in.ProfileFields[0].Id.String() + `"}]}]}]}`
	if kept, err := ParseDossier(good, in); err != nil || len(kept) != 1 {
		t.Fatalf("the parser kept %d sections of the well-formed reply (%v), want 1", len(kept), err)
	}
	if err := schema.ValidateJSON(req.ResponseSchema, good); err != nil {
		t.Errorf("the schema refuses a reply the parser keeps: %v", err)
	}
	invented := strings.Replace(good, `"kind":"summary"`, `"kind":"gossip"`, 1)
	if err := schema.ValidateJSON(req.ResponseSchema, invented); err == nil {
		t.Error("the schema admits a section kind the contract does not declare")
	}
}
