// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Every shipped language answers in ITS OWN words.
//
// A census that only checked for an entry would pass against a set copied from
// English and never translated — which is the same reader in the same wrong
// language, with a row in a table saying otherwise. Difference is the thing
// worth asserting, because sameness is what the bug looked like.

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

func TestNoShippedLanguageAnswersInEnglishByAccident(t *testing.T) {
	english := onboardingCopyByLang[textlang.English]
	for _, lang := range textlang.Shipped {
		if lang == textlang.English {
			continue
		}
		t.Run(string(lang), func(t *testing.T) {
			said := onboardingCopyByLang[lang]
			for _, field := range []struct{ name, got, en string }{
				{"entityQuestion", said.entityQuestion, english.entityQuestion},
				{"addressQuestion", said.addressQuestion, english.addressQuestion},
				{"conflictQuestion", said.conflictQuestion, english.conflictQuestion},
				{"keepLabel", said.keepLabel, english.keepLabel},
				{"keepDetail", said.keepDetail, english.keepDetail},
				{"takeLabel", said.takeLabel, english.takeLabel},
				{"takeDetail", said.takeDetail, english.takeDetail},
				{"statusConfirmed", said.statusConfirmed, english.statusConfirmed},
				{"statusFailed", said.statusFailed, english.statusFailed},
				{"statusResearching", said.statusResearching, english.statusResearching},
				{"statusMissing", said.statusMissing, english.statusMissing},
				{"selectionRecorded", said.selectionRecorded, english.selectionRecorded},
				{"selectionReason", said.selectionReason, english.selectionReason},
			} {
				if field.got == "" {
					t.Errorf("%s is empty — a blank line of copy reaches the reader as nothing at all", field.name)
					continue
				}
				if field.got == field.en {
					t.Errorf("%s is the English string verbatim; the entry exists and the reader still "+
						"gets English", field.name)
				}
			}
		})
	}
}

// The verbs behind the two buttons are wire values, not prose. A reader
// comparing what they clicked with what the record says needs the same word in
// both places, so these stay in English in every language.
func TestTheChoiceVerbsAreNotTranslated(t *testing.T) {
	for _, lang := range textlang.Shipped {
		said := onboardingCopyByLang[lang]
		for _, want := range []struct{ verb, in string }{
			{"keep_current", said.keepDetail},
			{"accept_proposal", said.takeDetail},
		} {
			if !strings.Contains(want.in, want.verb) {
				t.Errorf("%s's copy does not carry %q: %q", lang, want.verb, want.in)
			}
		}
	}
}

// The formatting holes have to survive translation. A %d dropped from one
// language's string prints the sentence without the number the sentence is
// about; a %s dropped from the conflict question stops naming the field.
func TestEveryLanguageKeepsItsFormattingHoles(t *testing.T) {
	for _, lang := range textlang.Shipped {
		said := onboardingCopyByLang[lang]
		for _, field := range []struct{ name, got, verb string }{
			{"conflictQuestion", said.conflictQuestion, "%s"},
			{"statusFailed", said.statusFailed, "%d"},
			{"statusMissing", said.statusMissing, "%d"},
		} {
			if strings.Count(field.got, field.verb) != 1 {
				t.Errorf("%s's %s carries %d %s verbs, want exactly 1: %q",
					lang, field.name, strings.Count(field.got, field.verb), field.verb, field.got)
			}
		}
	}
}
