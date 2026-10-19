// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import "testing"

func TestAKnownUnitSpellingIsStoredAsItsKey(t *testing.T) {
	t.Parallel()
	for sent, want := range map[string]string{
		"Stunde":    "hour",
		"HOURS":     "hour",
		"Tag":       "day",
		"unit":      "piece",
		"Tháng":     "month",
		"Pauschale": "flat_fee",
		"day":       "day",
		"Lizenz":    "Lizenz",
	} {
		if got := canonicalUnit(sent); got != want {
			t.Errorf("canonicalUnit(%q) = %q, want %q", sent, got, want)
		}
	}
}

func TestALineOrProductNamingNoUnitGetsTheDefaultKey(t *testing.T) {
	t.Parallel()
	if unit, _, _ := normalizeLineDefaults(nil, nil, nil); unit != defaultUnit {
		t.Fatalf("no unit stored %q, want %q", unit, defaultUnit)
	}
	blank := "   "
	if unit, _, _ := normalizeLineDefaults(&blank, nil, nil); unit != defaultUnit {
		t.Fatalf("a blank unit stored %q, want %q", unit, defaultUnit)
	}
	sent := " Stunden "
	if unit, _, _ := normalizeLineDefaults(&sent, nil, nil); unit != "hour" {
		t.Fatalf("%q stored %q, want hour", sent, unit)
	}
}

func TestALinePatchStoresAKnownUnitAsItsKey(t *testing.T) {
	t.Parallel()
	sent := " Tage "
	got, err := requireLineWords(UpdateOfferLineInput{Unit: &sent})
	if err != nil {
		t.Fatal(err)
	}
	if got.Unit == nil || *got.Unit != "day" {
		t.Fatalf("patch unit %v, want day", got.Unit)
	}
}
