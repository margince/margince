// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package langcopytest certifies that a copy table is written in every language
// the product ships.
//
// It is a package rather than a helper copied into each table's test because
// the check is the same everywhere and the failure it catches is silent: Go
// fills an omitted field of a keyed literal with "", so an untranslated
// sentence goes MISSING from a card rather than arriving in the wrong language.
// A census written out per package is a census free to check less in one of
// them, which is the shape of an under-reading gate that reports PASS.
package langcopytest

import (
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/langcopy"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// verbs counts the format placeholders in a template. A translation that drops
// one does not fail to compile — it renders "%!s(MISSING)" into a card — and a
// translation that adds one consumes an argument nobody passed.
var verbs = regexp.MustCompile(`%[a-zA-Z]|%%`)

// Census fails unless every Phrase field of table is written in every shipped
// language with the placeholders its English sentence was given.
//
// It refuses a table it cannot read rather than certifying nothing: a struct
// with no Phrase fields passes every assertion below without making a claim,
// which is how a census comes to report PASS over a subject it never saw.
//
//craft:ignore naked-any every caller passes a different copy table's own struct type — the parameter names the shape this walks, not a shape of ours
func Census(t *testing.T, table any) {
	t.Helper()
	shape := reflect.TypeOf(table)
	if shape == nil || shape.Kind() != reflect.Struct {
		t.Fatalf("a census needs a struct of phrases, got %T", table)
	}
	value := reflect.ValueOf(table)
	phrases := 0
	for i := range shape.NumField() {
		name := shape.Field(i).Name
		p, ok := value.Field(i).Interface().(langcopy.Phrase)
		if !ok {
			t.Errorf("%s is not a phrase, so this census says nothing about it", name)
			continue
		}
		phrases++
		english := p.In(textlang.English)
		if strings.TrimSpace(english) == "" {
			t.Errorf("%s has no English sentence, so there is nothing to translate against", name)
			continue
		}
		for _, lang := range textlang.Shipped {
			text := p.In(lang)
			if strings.TrimSpace(text) == "" {
				t.Errorf("%s leaves %s unwritten, which renders as a missing sentence rather than a wrong one",
					lang, name)
				continue
			}
			got, want := verbs.FindAllString(text, -1), verbs.FindAllString(english, -1)
			if !reflect.DeepEqual(got, want) {
				t.Errorf("%s writes %s with placeholders %v, but the sentence is given %v.\n  %s\n"+
					"A dropped placeholder renders as %%!s(MISSING) in a card; an extra one reads an "+
					"argument nobody passed.", lang, name, got, want, text)
			}
		}
	}
	if phrases == 0 {
		t.Fatal("the table holds no phrases; this census would certify nothing")
	}
}

// NoCount fails unless each named phrase is written without a placeholder, for
// the sentences that speak about exactly one of something. "1 days" and
// "1 Tagen" both read as a machine talking, and the second is not German.
func NoCount(t *testing.T, singulars map[string]langcopy.Phrase) {
	t.Helper()
	for _, lang := range textlang.Shipped {
		for name, p := range singulars {
			if got := verbs.FindAllString(p.In(lang), -1); len(got) != 0 {
				t.Errorf("%s writes %s with %v — a sentence about exactly one takes no count", lang, name, got)
			}
		}
	}
}
