// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package langcopytest

import (
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

func TestLegacyCensusRejectsIncompleteTranslationsAndContracts(t *testing.T) {
	type phraseFixture struct{ sentence string }
	valid := map[textlang.Lang]phraseFixture{
		textlang.English: {"Hello %s"}, textlang.German: {"Hallo %s"}, textlang.Vietnamese: {"Chào %s"},
	}
	for _, tc := range []struct{ name, german, want string }{
		{"empty", "", "de.sentence is empty"},
		{"untranslated", "Hello %s", "de.sentence repeats English"},
		{"missing placeholder", "Hallo", "de.sentence has placeholders"},
		{"changed placeholder", "Hallo %d", "de.sentence has placeholders"},
		{"indexed placeholder", "Hallo %[1]s", "de.sentence has placeholders"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			table := maps.Clone(valid)
			table[textlang.German] = phraseFixture{tc.german}
			got := run(func(r reporter) { CensusByLanguage(r, table, map[string][]string{"sentence": {"%s"}}) })
			if !reports(got.errors, tc.want) {
				t.Fatalf("got %v, want %s", got.errors, tc.want)
			}
		})
	}
	got := run(func(r reporter) { CensusByLanguage(r, valid, map[string][]string{"sentence": {"%s"}}) })
	if got.fatal != "" || len(got.errors) != 0 {
		t.Fatalf("valid copy failed: %+v", got)
	}
	missing := run(func(r reporter) { CensusByLanguage(r, valid, map[string][]string{"old": nil}) })
	if !reports(missing.errors, "no call-site") || !reports(missing.errors, "no sentence") {
		t.Fatalf("short contract passed: %+v", missing)
	}
	delete(valid, textlang.German)
	missing = run(func(r reporter) { CensusByLanguage(r, valid, map[string][]string{"sentence": {"%s"}}) })
	if !reports(missing.errors, "no de entry") {
		t.Fatalf("missing language passed: %+v", missing)
	}
	delete(valid, textlang.English)
	missing = run(func(r reporter) { CensusByLanguage(r, valid, nil) })
	if !strings.Contains(missing.fatal, "no English") {
		t.Fatalf("missing baseline passed: %+v", missing)
	}
}

func TestLegacySentencesRejectShortCensuses(t *testing.T) {
	for _, tc := range []struct {
		name string
		call func(reporter)
		want string
	}{
		{"wrong shape", func(r reporter) { Sentences(r, nil) }, "not a struct"},
		{"empty", func(r reporter) { Sentences(r, struct{}{}) }, "no sentences"},
		{"unknown field", func(r reporter) {
			Sentences(r, struct {
				Text  string
				Count int
			}{"Hi", 1})
		}, "Count is not a sentence"},
		{"stale exclusion", func(r reporter) { Sentences(r, struct{ Text string }{"Hi"}, "old") }, "old does not exist"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := run(tc.call)
			if !strings.Contains(got.fatal, tc.want) && !reports(got.errors, tc.want) {
				t.Fatalf("short census passed: %+v", got)
			}
		})
	}
	got := run(func(r reporter) {
		Sentences(r, struct {
			Text  string
			Count int
		}{"Hi", 1}, "Count")
	})
	if got.fatal != "" || len(got.errors) != 0 {
		t.Fatalf("explicit metadata failed: %+v", got)
	}
}

func TestStyleByLanguageHoldsEachLanguageInItsRegister(t *testing.T) {
	type copyFixture struct{ Saved string }
	structs := map[textlang.Lang]copyFixture{
		textlang.English: {"We saved it"}, textlang.German: {"Gespeichert"}, textlang.Vietnamese: {"Đã lưu — xong"},
	}
	if got := rulesReported(func(r reporter) { StyleByLanguage(r, structs) }); !slices.Equal(got, []string{ruleEnWe, ruleViDash}) {
		t.Errorf("a struct table as member copy reported %v, want [%s %s]", got, ruleEnWe, ruleViDash)
	}
	controller := Controller("a fixture the data controller writes")
	if got := rulesReported(func(r reporter) { StyleByLanguage(r, structs, controller) }); !slices.Equal(got, []string{ruleViDash}) {
		t.Errorf("a struct table in the controller's voice reported %v, want [%s]", got, ruleViDash)
	}
	lists := map[textlang.Lang][]string{textlang.English: {"Saved", "Saved — done"}, textlang.Vietnamese: {"Đã lưu"}}
	got := run(func(r reporter) { StyleByLanguage(r, lists) })
	if !reports(got.errors, "en 1 ") || !reports(got.errors, "no de entry") || len(got.errors) != 2 {
		t.Errorf("a list table missing German reported %v, want its second English line and the missing entry", got.errors)
	}
}
