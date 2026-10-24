// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"strings"
	"testing"
	"unicode/utf8"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy"
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy/langcopytest"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

func TestTextThatNamesAnIndustryResolvesToItsCode(t *testing.T) {
	for raw, want := range map[string]string{
		"Healthcare":                "healthcare",
		"health care":               "healthcare",
		"HEALTHCARE":                "healthcare",
		"Gesundheitswesen":          "healthcare",
		"Marketing And Advertising": "marketing_advertising",
		"marketing_advertising":     "marketing_advertising",
		"Sản xuất":                  "manufacturing",
		"Maschinenbau":              "manufacturing",
		"  Software  ":              "software",
	} {
		got, ok := ResolveIndustry(raw)
		if !ok || got != want {
			t.Errorf("ResolveIndustry(%q) = %q, %v, want %q", raw, got, ok, want)
		}
	}
}

func TestTextThatNamesNoIndustryResolvesToNothing(t *testing.T) {
	for _, raw := range []string{"", "   ", "pumps, valves, gaskets and seals", "Other"} {
		if code, ok := ResolveIndustry(raw); ok {
			t.Errorf("ResolveIndustry(%q) = %q, want no code", raw, code)
		}
	}
}

func TestNoTwoIndustriesClaimOneSpelling(t *testing.T) {
	if _, clashes := industryKeys(industryTable); len(clashes) > 0 {
		t.Errorf("spellings claimed by two codes: %v", clashes)
	}
}

func TestEveryIndustryCodeIsWordedInEveryLanguage(t *testing.T) {
	codes := IndustryCodes()
	if len(codes) != 26 {
		t.Fatalf("the list has %d codes, want the 26 decided", len(codes))
	}
	for _, code := range codes {
		for _, lang := range textlang.Shipped {
			if IndustryLabel(code, lang) == "" {
				t.Errorf("%s has no %s label", code, lang)
			}
		}
		if got, ok := ResolveIndustry(IndustryLabel(code, textlang.German)); !ok || got != code {
			t.Errorf("the German label of %s resolves to %q", code, got)
		}
	}
}

func TestTheIndustryLabelsPassTheLanguageCensus(t *testing.T) {
	labels := make(map[string]langcopy.Phrase, len(industryTable))
	for _, e := range industryTable {
		labels[string(e.code)] = e.label
	}
	langcopytest.Census(t, labels)
}

func TestAReaderSeesALabelForACodeAndTheStoredValueForAnythingElse(t *testing.T) {
	if got := IndustryWords("healthcare", textlang.German); got != "Gesundheitswesen" {
		t.Errorf("code worded %q", got)
	}
	if got := IndustryWords("Medical devices", textlang.German); got != "Medical devices" {
		t.Errorf("legacy text worded %q, want it as stored", got)
	}
	if got := IndustryLabel("Medical devices", textlang.English); got != "" {
		t.Errorf("a non-code was labelled %q", got)
	}
}

func TestTheIndustryNoteIsAppendedOnceAndKeepsTheTextBeforeIt(t *testing.T) {
	description := new("Maker of industrial pumps")
	next, changed := withIndustrySource(description, "pumps and valves")
	if !changed || next != "Maker of industrial pumps Industry as written: pumps and valves" {
		t.Fatalf("first call = %q, %v", next, changed)
	}
	again, changed := withIndustrySource(&next, "pumps and valves")
	if changed || again != next {
		t.Errorf("second call = %q, %v, want no change", again, changed)
	}
	if _, changed := withIndustrySource(description, "  "); changed {
		t.Error("an empty raw text added a note")
	}
	fromNothing, changed := withIndustrySource(nil, "pumps")
	if !changed || fromNothing != "Industry as written: pumps" {
		t.Errorf("note on an empty description = %q, %v", fromNothing, changed)
	}
}

func TestTheIndustryNoteNeverPassesTheDescriptionColumn(t *testing.T) {
	full := strings.Repeat("a", CompanyDescriptionMax-5)
	if next, changed := withIndustrySource(&full, "pumps"); changed || next != full {
		t.Errorf("a note that would pass the column was added: %d runes", utf8.RuneCountInString(next))
	}
	long := strings.Repeat("x", 300)
	next, changed := withIndustrySource(nil, long)
	if !changed || utf8.RuneCountInString(next) > len(industryNotePrefix)+industryNoteRunes {
		t.Errorf("the raw text was not cut: %d runes", utf8.RuneCountInString(next))
	}
}

func TestEveryColumnWriterBindsTheIndustryCode(t *testing.T) {
	if got, unmapped := companyColumnArgument(columnIndustry, "health care"); unmapped || got == nil || *got != "healthcare" {
		t.Errorf("mapped text bound %v, %v", got, unmapped)
	}
	if got, unmapped := companyColumnArgument(columnIndustry, "pumps and valves"); !unmapped || got != nil {
		t.Errorf("unmapped text bound %v, %v, want nothing and unmapped", got, unmapped)
	}
	if got, unmapped := companyColumnArgument(columnIndustry, ""); unmapped || got != nil {
		t.Errorf("empty bound %v, %v, want NULL", got, unmapped)
	}
	if got, _ := companyColumnArgument(columnLegalName, "Acme"); got == nil || *got != "Acme" {
		t.Errorf("another column bound %v", got)
	}
}

func TestAPatchStoresTheCodeAndAnUnmappableTextOnlyAddsTheNote(t *testing.T) {
	standing := crmcontracts.CompanyIndustryHealthcare
	current := crmcontracts.Company{Industry: &standing, Description: new("Maker of pumps")}

	mapped := storekit.NewPatch()
	setCompanyIndustry(mapped, current, UpdateCompanyInput{Industry: new("Software")})
	if got := mapped.After()["industry"]; got != "software" {
		t.Errorf("mapped text staged %v", got)
	}

	unmappable := storekit.NewPatch()
	setCompanyIndustry(unmappable, current, UpdateCompanyInput{Industry: new("pumps and valves")})
	if _, staged := unmappable.After()["industry"]; staged {
		t.Error("an unmappable cell touched a standing code")
	}
	if got := unmappable.After()["description"]; got != "Maker of pumps Industry as written: pumps and valves" {
		t.Errorf("description staged %v", got)
	}

	sent := storekit.NewPatch()
	setCompanyIndustry(sent, current, UpdateCompanyInput{Industry: new("pumps"), Description: new("New words")})
	if got := sent.After()["description"]; got != "New words Industry as written: pumps" {
		t.Errorf("note was folded into %v, want the description the caller sent", got)
	}

	cleared := storekit.NewPatch()
	setCompanyIndustry(cleared, current, UpdateCompanyInput{Industry: new("")})
	if got, staged := cleared.After()["industry"]; !staged || got != nil {
		t.Errorf("a blank industry staged %v, %v, want NULL", got, staged)
	}
}

func TestMergeGivesTheSurvivorTheCodeOrTheNote(t *testing.T) {
	legacy := crmcontracts.CompanyIndustry("health care")
	p := buildCompanySurvivorshipPatch(crmcontracts.Company{}, crmcontracts.Company{Industry: &legacy})
	if got := p.After()["industry"]; got != "healthcare" {
		t.Errorf("legacy text merged as %v", got)
	}
	free := crmcontracts.CompanyIndustry("pumps and valves")
	p = buildCompanySurvivorshipPatch(crmcontracts.Company{}, crmcontracts.Company{Industry: &free, Description: new("Pumps")})
	if _, staged := p.After()["industry"]; staged {
		t.Error("unmappable text was merged as a code")
	}
	if got := p.After()["description"]; got != "Pumps Industry as written: pumps and valves" {
		t.Errorf("description merged as %v", got)
	}
	standing := crmcontracts.CompanyIndustrySoftware
	p = buildCompanySurvivorshipPatch(crmcontracts.Company{Industry: &standing}, crmcontracts.Company{Industry: &legacy})
	if _, staged := p.After()["industry"]; staged {
		t.Error("the survivor's own industry was replaced")
	}
}

func TestACreateStoresTheCodeOrKeepsTheTextInTheDescription(t *testing.T) {
	industry, description := CompanyIndustryColumns(new("Healthcare"), new("Clinics"))
	if industry == nil || *industry != "healthcare" || *description != "Clinics" {
		t.Errorf("mapped create = %v, %v", industry, description)
	}
	industry, description = CompanyIndustryColumns(new("pumps"), nil)
	if industry != nil || description == nil || *description != "Industry as written: pumps" {
		t.Errorf("unmapped create = %v, %v", industry, description)
	}
	industry, description = CompanyIndustryColumns(nil, new("Clinics"))
	if industry != nil || *description != "Clinics" {
		t.Errorf("silent create = %v, %v", industry, description)
	}
}
