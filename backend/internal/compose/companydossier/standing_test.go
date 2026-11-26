// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package companydossier

import (
	"context"
	"reflect"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// A company with no profile rows but a description, a lifecycle, contacts and
// open deals: a record the overview must not call empty.
func recordedCompany() Input {
	return Input{
		CompanyID: ids.NewV7().String(),
		Industry:  "software",
		SizeBand:  "51-200",
		Standing: Standing{
			Description: "Load-shifting software for factories",
			Lifecycle:   "customer",
			Contacts:    new(3),
			OpenDeals:   new(2),
		},
	}
}

func floorTexts(in Input, lang string) map[string][]string {
	out := map[string][]string{}
	for _, section := range keepGrounded(Deterministic(in, lang), in) {
		for _, sentence := range section.Sentences {
			out[section.Kind] = append(out[section.Kind], sentence.Text)
		}
	}
	return out
}

func TestTheFloorStatesTheCompanyRecordInTheReadersLanguage(t *testing.T) {
	want := map[string][]string{
		sectionSummary: {
			"Was sie tun: Load-shifting software for factories.",
			"Lebenszyklus: Kunde.", "Kontakte: 3.", "Offene Deals: 2.",
		},
		sectionMarkets:       {"Branche: Software."},
		sectionFirmographics: {"Mitarbeitende: 51–200."},
	}
	if got := floorTexts(recordedCompany(), "de"); !reflect.DeepEqual(got, want) {
		t.Errorf("the floor of a company with no profile rows = %v, want %v", got, want)
	}
}

func TestACompanyWithNothingRecordedStillReadsAsEmpty(t *testing.T) {
	in := Input{
		CompanyID: ids.NewV7().String(),
		Standing:  Standing{Lifecycle: "", Contacts: new(0), OpenDeals: new(0)},
	}
	if got := floorTexts(in, "en"); len(got) != 0 {
		t.Errorf("a company with none of the core fields got %v, want no section", got)
	}
}

func TestACountTheReaderMayNotSeeIsNotStated(t *testing.T) {
	in := recordedCompany()
	in.Standing.OpenDeals = nil
	for _, text := range floorTexts(in, "en")[sectionSummary] {
		if text == "Open deals: 2." {
			t.Errorf("a withheld count was stated: %q", text)
		}
	}
}

func TestEveryLifecycleButUnknownIsWorded(t *testing.T) {
	for _, stage := range values.ClassifiedLifecycles() {
		_, worded := lifecycleWords[crmcontracts.CompanyStatus(stage)]
		if worded == (stage == string(crmcontracts.CompanyStatusUnknown)) {
			t.Errorf("lifecycle %q worded = %v; every stage but unknown needs words", stage, worded)
		}
	}
}

func TestTheStandingIsReadFromTheCompanyRowAndLeavesUnknownOut(t *testing.T) {
	want := Standing{Description: "Clinics", Contacts: new(4), OpenDeals: new(1)}
	stub := stubFacts{in: Input{Standing: Standing{
		Description: " Clinics ", Lifecycle: "unknown", Contacts: new(4), OpenDeals: new(1),
	}}}
	got, err := readStanding(context.Background(), stub, ids.From[ids.CompanyKind](ids.NewV7()))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("standing = %+v, want %+v", got, want)
	}
}
