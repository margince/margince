// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Every shipped language reads a deterministic signal in ITS OWN words.
//
// A census that only checked for an entry would pass against a set copied from
// English and never translated — the same reader in the same wrong language,
// with a row in a table saying otherwise. So the sentences are asserted
// different from English, and their formatting holes asserted present: a %d
// dropped in translation prints a sentence without the number the sentence is
// about, and a %s dropped from the project line stops naming the project.

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

func TestEveryShippedLanguageWritesItsOwnSignalSummaries(t *testing.T) {
	english, ok := signalSummaryByLang[textlang.English]
	if !ok {
		t.Fatal("English has no signal summary set, and it is the fallback for everything else")
	}
	for _, lang := range textlang.Shipped {
		t.Run(string(lang), func(t *testing.T) {
			said, ok := signalSummaryByLang[lang]
			if !ok {
				t.Fatalf("the product ships %s and this table has not learned it", lang)
			}
			for _, field := range []struct {
				name, got, en string
				holes         map[string]int
			}{
				{"ghostedThread", said.ghostedThread, english.ghostedThread, map[string]int{"%d": 1}},
				{"projectQuiet", said.projectQuiet, english.projectQuiet, map[string]int{"%s": 1, "%d": 1}},
			} {
				if field.got == "" {
					t.Errorf("%s is empty — a blank summary reaches the reader as nothing at all", field.name)
					continue
				}
				for verb, want := range field.holes {
					if got := strings.Count(field.got, verb); got != want {
						t.Errorf("%s carries %d %s holes, want %d: %q", field.name, got, verb, want, field.got)
					}
				}
				if lang != textlang.English && field.got == field.en {
					t.Errorf("%s is the English sentence verbatim; the entry exists and the reader still gets English",
						field.name)
				}
			}
		})
	}
}

// The project line names the project before it counts the days. Go's verbs are
// positional, so a translation that reorders them silently prints the day count
// as the project's name.
func TestTheProjectSummaryNamesTheProjectBeforeTheDayCount(t *testing.T) {
	for _, lang := range textlang.Shipped {
		said := signalSummaryByLang[lang]
		if strings.Index(said.projectQuiet, "%s") > strings.Index(said.projectQuiet, "%d") {
			t.Errorf("%s's projectQuiet counts days before it names the project, so the two arguments swap: %q",
				lang, said.projectQuiet)
		}
	}
}

// An unshipped language answers English rather than an empty set, because the
// language comes off a settings row a person can edit by hand.
func TestAnUnknownLanguageFallsBackToEnglish(t *testing.T) {
	said := signalSummaryCopyFor(textlang.Lang("kl"))
	if said.ghostedThread != signalSummaryByLang[textlang.English].ghostedThread {
		t.Errorf("an unshipped language answered %q, want the English sentence", said.ghostedThread)
	}
}
