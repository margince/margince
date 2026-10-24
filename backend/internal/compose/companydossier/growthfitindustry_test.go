// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package companydossier

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

func missingCodes(t *testing.T, in Input) []crmcontracts.GrowthFitInput {
	t.Helper()
	got := Completeness(in, assessedAt)
	if got.Missing == nil {
		t.Fatal("missing is nil")
	}
	return *got.Missing
}

// A company that shows an industry is never listed as missing it.
// A column has no age and a site-read row does.
func TestAnIndustryTheCompanyShowsIsNeverListedAsMissing(t *testing.T) {
	fresh := assessedAt.Add(-24 * time.Hour)
	stale := assessedAt.Add(-90 * 24 * time.Hour)
	cases := map[string]struct {
		column       string
		sidecar      []crmcontracts.CompanyProfileField
		wantMissing  bool
		wantNoExpiry bool
	}{
		"column only":                  {column: "healthcare", wantNoExpiry: true},
		"sidecar fresh only":           {sidecar: []crmcontracts.CompanyProfileField{machineField("industry", "Medical devices", fresh)}},
		"sidecar stale only":           {sidecar: []crmcontracts.CompanyProfileField{machineField("industry", "Medical devices", stale)}, wantMissing: true},
		"neither":                      {wantMissing: true},
		"column set and sidecar stale": {column: "healthcare", sidecar: []crmcontracts.CompanyProfileField{machineField("industry", "x", stale)}, wantNoExpiry: true},
		"blank column":                 {column: "   ", wantMissing: true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			in := Input{Industry: tc.column, ProfileFields: tc.sidecar}
			counted, staleAt := completenessAt(in, assessedAt)
			missing := slices.Contains(*counted.Missing, crmcontracts.GrowthFitInputIndustry)
			if missing != tc.wantMissing {
				t.Errorf("industry missing = %v, want %v (missing: %v)", missing, tc.wantMissing, *counted.Missing)
			}
			if tc.wantNoExpiry && !staleAt.IsZero() {
				t.Errorf("the column gave an expiry %v, want none", staleAt)
			}
		})
	}
}

// The wire carries the seven codes in table order.
// A code the client has no wording for is then a contract change, not a drift.
func TestAMissingInputIsSentAsItsCodeAndNotAsEnglish(t *testing.T) {
	got := missingCodes(t, Input{})
	want := []crmcontracts.GrowthFitInput{
		crmcontracts.GrowthFitInputOfferSummary, crmcontracts.GrowthFitInputIcp, crmcontracts.GrowthFitInputIndustry,
		crmcontracts.GrowthFitInputBuyingCenter, crmcontracts.GrowthFitInputBuyingIntents,
		crmcontracts.GrowthFitInputEmployeeRange, crmcontracts.GrowthFitInputTechnology,
	}
	if !slices.Equal(got, want) {
		t.Errorf("missing = %v, want %v", got, want)
	}
	for _, code := range got {
		if !code.Valid() {
			t.Errorf("%q is no code of the contract", code)
		}
	}
}

func TestEachNextStepIsACode(t *testing.T) {
	below := Assess(Input{}, crmcontracts.GrowthFitBandStrong, true, AbstainedNoWriter, assessedAt)
	if below.NextStep != crmcontracts.GrowthFitNextStepGatherInputs {
		t.Errorf("below the floor next step = %q, want gather_inputs", below.NextStep)
	}
	unconfirmed := Assess(fourOfSeven(), crmcontracts.GrowthFitBandStrong, false, AbstainedNoWriter, assessedAt)
	if unconfirmed.NextStep != crmcontracts.GrowthFitNextStepConfirmOffering {
		t.Errorf("unconfirmed next step = %q, want confirm_offering", unconfirmed.NextStep)
	}
	noWriter := Assess(sevenOfSeven(), crmcontracts.GrowthFitBandUnknown, true, AbstainedNoWriter, assessedAt)
	if noWriter.NextStep != crmcontracts.GrowthFitNextStepConfigureModel {
		t.Errorf("no writer next step = %q, want configure_model", noWriter.NextStep)
	}
	failed := Assess(sevenOfSeven(), crmcontracts.GrowthFitBandUnknown, true, AbstainedLaneFailed, assessedAt)
	if failed.NextStep != crmcontracts.GrowthFitNextStepRetryLater {
		t.Errorf("failed lane next step = %q, want retry_later", failed.NextStep)
	}
	if got := gatherNextStep(crmcontracts.DataCompleteness{}); got != crmcontracts.GrowthFitNextStepRecordProfile {
		t.Errorf("nothing named next step = %q, want record_profile", got)
	}
}

func TestTheModelReadsTheColumnIndustryWordedInThePromptLanguage(t *testing.T) {
	in := Input{Industry: "healthcare"}
	for lang, want := range map[textlang.Lang]string{
		textlang.English: "Healthcare", textlang.German: "Gesundheitswesen", textlang.Vietnamese: "Y tế",
	} {
		var sent modelInput
		if err := json.Unmarshal([]byte(encodeInput(in, string(lang))), &sent); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if sent.Industry != want {
			t.Errorf("%s: industry = %q, want %q", lang, sent.Industry, want)
		}
	}
	var unrepaired modelInput
	if err := json.Unmarshal([]byte(encodeInput(Input{Industry: "Medical devices"}, "en")), &unrepaired); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if unrepaired.Industry != "Medical devices" {
		t.Errorf("a value that is no code was sent as %q, want it as stored", unrepaired.Industry)
	}
	if strings.Contains(encodeInput(Input{}, "en"), `"industry"`) {
		t.Error("an empty industry reached the model input")
	}
}

func TestChangingTheColumnIndustryMovesBothFingerprints(t *testing.T) {
	without := fourOfSeven()
	with := fourOfSeven()
	with.CompanyID = without.CompanyID
	with.Industry = "healthcare"
	offering := Offering{Confirmed: true, Fingerprint: "o"}
	lang := string(textlang.English)

	a, err := growthFitFingerprint(without, "r", offering, lang)
	if err != nil {
		t.Fatal(err)
	}
	b, err := growthFitFingerprint(with, "r", offering, lang)
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Error("the growth-fit fingerprint ignored the industry column")
	}
	c, err := Fingerprint(without, "r", lang)
	if err != nil {
		t.Fatal(err)
	}
	d, err := Fingerprint(with, "r", lang)
	if err != nil {
		t.Fatal(err)
	}
	if c == d {
		t.Error("the dossier fingerprint ignored the industry column")
	}
	if encoded, _ := json.Marshal(without); strings.Contains(string(encoded), `"industry":`) {
		t.Errorf("a company without the column hashes an industry key, so its fingerprint moved: %s", encoded)
	}
}

func TestAStoredAssessmentOfTheEnglishSentenceShapeIsNotUsable(t *testing.T) {
	old := storedGrowthFit{Version: 2, Fingerprint: "abc"}
	if old.usable("abc", assessedAt) {
		t.Error("a version 2 payload, holding English labels, was served")
	}
	if old.mayStandIn(true, "abc", assessedAt) {
		t.Error("a version 2 payload stood in for a failed lane")
	}
}

func TestBuildInputCarriesTheCompanyColumnIndustry(t *testing.T) {
	facts := stubFacts{in: Input{Name: "Acme", Industry: " healthcare "}}
	in, err := BuildInput(context.Background(), facts, ids.New[ids.CompanyKind]())
	if err != nil {
		t.Fatalf("BuildInput: %v", err)
	}
	if in.Industry != "healthcare" {
		t.Errorf("industry = %q, want the trimmed column value", in.Industry)
	}
}
