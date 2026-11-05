// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// Every field update_record may change has a label in every shipped language,
// and the table names no field that no update shape has.
func TestEveryUpdatableFieldHasALabelInEveryLanguage(t *testing.T) {
	updatable := map[string]bool{}
	for _, shape := range updateShapes {
		for _, field := range contractFieldNames(shape) {
			updatable[field] = true
		}
	}
	if len(updatable) == 0 {
		t.Fatal("the update shapes name no field, so this census would pass vacuously")
	}
	for _, lang := range textlang.Shipped {
		labels, ok := fieldLabelsByLang[lang]
		if !ok {
			t.Errorf("%s has no field labels; an approval in that language names fields by wire name", lang)
			continue
		}
		for field := range updatable {
			if strings.TrimSpace(labels[field]) == "" {
				t.Errorf("%s: field %q has no label; an approval shows it by its wire name", lang, field)
			}
		}
		for field := range labels {
			if !updatable[field] {
				t.Errorf("%s: %q is labelled but no update shape has it", lang, field)
			}
		}
	}
}

func TestFieldLabelsNameFieldsInTheReadersLanguage(t *testing.T) {
	fields := []string{"amount_minor", "expected_close_date", "cf_contract_tier"}
	cases := map[textlang.Lang]string{
		textlang.English: "Value, Expected close, Contract tier",
		textlang.German:  "Wert, Voraussichtlicher Abschluss, Contract tier",
		textlang.Unknown: "Value, Expected close, Contract tier",
	}
	for lang, want := range cases {
		if got := FieldLabels(lang, fields); got != want {
			t.Errorf("FieldLabels(%s) = %q, want %q", lang, got, want)
		}
	}
	if got := LabelledFields(); !slices.IsSorted(got) || !slices.Contains(got, "owner_id") {
		t.Errorf("LabelledFields() = %v, want the sorted table keys", got)
	}
}
