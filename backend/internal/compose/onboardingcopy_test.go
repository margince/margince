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
	"reflect"
	"regexp"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

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

// germanField is one hand-written German string and the field it came from.
type germanField struct{ name, text string }

// germanOnboarding reads the German entry off the struct rather than listing
// its fields, so copy added to the conversation is addressed by the register
// gate below without anyone remembering to extend it.
func germanOnboarding(t *testing.T) []germanField {
	t.Helper()
	said := reflect.ValueOf(onboardingCopyByLang[textlang.German])
	fields := make([]germanField, 0, said.NumField())
	for i := range said.NumField() {
		// A field this loop cannot read is a field it skips silently, and a
		// census that reads a smaller subject than it has still says PASS.
		if said.Field(i).Kind() != reflect.String {
			t.Fatalf("onboardingCopy.%s holds no string, so this gate never reads it",
				said.Type().Field(i).Name)
		}
		fields = append(fields, germanField{said.Type().Field(i).Name, said.Field(i).String()})
	}
	if len(fields) == 0 {
		t.Fatal("the German onboarding copy carries no fields, so nothing below was checked")
	}
	return fields
}

// What a capital may follow while still being a capital only because a
// sentence starts there.
var sentenceOpens = regexp.MustCompile(`(?:[.!?:]\s|\n|[„"(])$`)

func opensASentence(text string, at int) bool {
	return at == 0 || sentenceOpens.MatchString(text[:at])
}

func isCapitalised(word string) bool {
	first, _ := utf8.DecodeRuneInString(word)
	return unicode.IsUpper(first)
}

// Every field here is spoken TO the reader, and this product's German speaks du.
// So a capitalised Sie or Ihr is the formal address with no second reading —
// unlike in prose, where one opening a sentence may still be she or they.
func TestTheGermanOnboardingCopyAddressesItsReaderAsDu(t *testing.T) {
	for _, field := range germanOnboarding(t) {
		for _, at := range textlang.SieForms().FindAllStringIndex(field.text, -1) {
			t.Errorf("%s addresses the reader formally as %q: %q",
				field.name, field.text[at[0]:at[1]], field.text)
		}
		for _, at := range textlang.DuForms().FindAllStringIndex(field.text, -1) {
			word := field.text[at[0]:at[1]]
			if !isCapitalised(word) || opensASentence(field.text, at[0]) {
				continue
			}
			t.Errorf("%s capitalises %q mid-sentence, which is the formal register wearing "+
				"the informal word: %q", field.name, word, field.text)
		}
	}
}
