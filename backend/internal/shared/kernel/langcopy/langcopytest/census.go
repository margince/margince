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
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"

	"github.com/margince/margince/backend/internal/shared/kernel/langcopy"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// verbs counts the format placeholders in a template. A translation that drops
// one does not fail to compile — it renders "%!s(MISSING)" into a card — and a
// translation that adds one consumes an argument nobody passed.
var verbs = regexp.MustCompile(`%[a-zA-Z]|%%`)

// reporter is the slice of *testing.T's methods this package calls. Narrower
// than testing.TB, whose unexported method rules out a test double.
type reporter interface {
	Helper()
	Errorf(format string, args ...any)
	Fatalf(format string, args ...any)
}

// Census fails unless every Phrase a table holds is written in every shipped
// language with the placeholders its English sentence was given.
//
// table is a struct, or a map keyed by whatever names its entries — a plain
// label table has no struct around it at all. A struct field may itself be a
// map of phrases, for a table that answers a stored field or activity kind
// rather than a fixed slot; walked entry by entry and reported
// "Field[key]", because "something in Field is unwritten" sends the reader
// grepping for which one.
//
// It refuses a table it cannot read rather than certifying nothing: a struct
// with no Phrase fields, or a table with no entries by either route, passes
// every assertion below without making a claim — which is how a census comes
// to report PASS over a subject it never saw.
//
// This is the one census every langcopy.Phrase table calls. A copy table
// written as a map keyed by language rather than a struct of phrases keeps its
// own walk until it is migrated onto Phrase.
//
// Held by: TestOneCensusReadsEveryPhraseTable (backend/gates/langcopyonecensus_test.go)
//
//craft:ignore naked-any every caller passes a different copy table's own struct or map type — the parameter names the shape this walks, not a shape of ours
func Census(t reporter, table any) {
	t.Helper()
	shape := reflect.TypeOf(table)
	value := reflect.ValueOf(table)
	phrases := 0
	switch {
	case shape != nil && shape.Kind() == reflect.Struct:
		for i := range shape.NumField() {
			phrases += censusField(t, shape.Field(i).Name, value.Field(i))
		}
	case shape != nil && shape.Kind() == reflect.Map:
		phrases += censusMap(t, "", value)
	default:
		t.Fatalf("a census needs a struct or map of phrases, got %T", table)
	}
	if phrases == 0 {
		t.Fatalf("the table holds no phrases; this census would certify nothing")
	}
}

// censusField reads one struct field, which is either a phrase directly or a
// map of them, and reports how many phrases it found.
func censusField(t reporter, name string, field reflect.Value) int {
	t.Helper()
	if p, ok := field.Interface().(langcopy.Phrase); ok {
		censusEntry(t, name, p)
		return 1
	}
	if field.Kind() == reflect.Map {
		return censusMap(t, name, field)
	}
	t.Errorf("%s is not a phrase, so this census says nothing about it", name)
	return 0
}

// censusMap walks a map of phrases in a fixed order, sorted rather than the
// map's own random iteration order — an unsorted walk reorders its failures
// between runs, and a reader comparing two runs cannot tell what changed.
// prefix names the enclosing field, empty for a table that is itself a map.
func censusMap(t reporter, prefix string, table reflect.Value) int {
	t.Helper()
	keys := table.MapKeys()
	sort.Slice(keys, func(i, j int) bool {
		return fmt.Sprint(keys[i].Interface()) < fmt.Sprint(keys[j].Interface())
	})
	phrases := 0
	for _, key := range keys {
		name := fmt.Sprintf("%s[%v]", prefix, key.Interface())
		p, ok := table.MapIndex(key).Interface().(langcopy.Phrase)
		if !ok {
			t.Errorf("%s is not a phrase, so this census says nothing about it", name)
			continue
		}
		censusEntry(t, name, p)
		phrases++
	}
	return phrases
}

// censusEntry holds the one check every phrase answers to, however it was
// reached: written in every shipped language, with the same placeholders its
// English sentence was given.
func censusEntry(t reporter, name string, p langcopy.Phrase) {
	t.Helper()
	english := p.In(textlang.English)
	if strings.TrimSpace(english) == "" {
		t.Errorf("%s has no English sentence, so there is nothing to translate against", name)
		return
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

// NoCount fails unless each named phrase is written without a placeholder, for
// the sentences that speak about exactly one of something. "1 days" and
// "1 Tagen" both read as a machine talking, and the second is not German.
func NoCount(t reporter, singulars map[string]langcopy.Phrase) {
	t.Helper()
	for _, lang := range textlang.Shipped {
		for name, p := range singulars {
			if got := verbs.FindAllString(p.In(lang), -1); len(got) != 0 {
				t.Errorf("%s writes %s with %v — a sentence about exactly one takes no count", lang, name, got)
			}
		}
	}
}
