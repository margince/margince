// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"maps"
	"testing"
)

func TestEverySeededStageCarriesItsGermanAndVietnameseName(t *testing.T) {
	ladder := append(append([]StageInput{}, defaultStages...), missingClosingStages(nil)...)
	for _, st := range ladder {
		if st.NameTranslations["de"] == "" || st.NameTranslations["vi"] == "" {
			t.Errorf("seeded stage %q has translations %v, want de and vi", st.Name, st.NameTranslations)
		}
		if st.NameTranslations["de"] == st.Name || st.NameTranslations["vi"] == st.Name {
			t.Errorf("seeded stage %q is translated as its English name: %v", st.Name, st.NameTranslations)
		}
	}
}

func TestASeededStageHandsOutACopyOfItsTranslations(t *testing.T) {
	seededTranslations("Won")["de"] = "Changed"
	if got := seededStageNames["Won"]["de"]; got != "Gewonnen" {
		t.Fatalf("the seeded German name of Won is %q after a caller changed its copy", got)
	}
}

func TestARenameDropsOnlyTheSeededTranslations(t *testing.T) {
	newName := "Contract sent"
	admin := map[string]string{"de": "Vertrag raus"}
	cases := []struct {
		name    string
		current stageConfig
		in      UpdateStageInput
		want    map[string]string
		changes bool
	}{
		{"seeded stage renamed alone", stageConfig{name: "Negotiation", translations: seededTranslations("Negotiation")},
			UpdateStageInput{Name: &newName}, map[string]string{}, true},
		{"seeded stage renamed with the seeded set sent back", stageConfig{name: "Negotiation", translations: seededTranslations("Negotiation")},
			UpdateStageInput{Name: &newName, NameTranslations: seededTranslations("Negotiation")}, map[string]string{}, true},
		{"seeded stage renamed with new translations", stageConfig{name: "Negotiation", translations: seededTranslations("Negotiation")},
			UpdateStageInput{Name: &newName, NameTranslations: admin}, admin, true},
		{"admin translations survive a rename", stageConfig{name: "Negotiation", translations: admin},
			UpdateStageInput{Name: &newName}, nil, false},
		{"no rename keeps the seeded set", stageConfig{name: "Negotiation", translations: seededTranslations("Negotiation")},
			UpdateStageInput{}, nil, false},
	}
	for _, c := range cases {
		got, changes := renamedTranslations(c.current, c.in)
		if changes != c.changes || (changes && !maps.Equal(got, c.want)) {
			t.Errorf("%s: renamedTranslations = %v, %v; want %v, %v", c.name, got, changes, c.want, c.changes)
		}
	}
}
