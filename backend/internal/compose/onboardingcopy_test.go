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
	"regexp"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/margince/margince/backend/internal/shared/kernel/langcopy/langcopytest"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

func TestNoShippedLanguageAnswersInEnglishByAccident(t *testing.T) {
	english := langcopytest.Sentences(t, onboardingCopyByLang[textlang.English])
	for _, lang := range textlang.Shipped {
		if lang == textlang.English {
			continue
		}
		t.Run(string(lang), func(t *testing.T) {
			for name, got := range langcopytest.Sentences(t, onboardingCopyByLang[lang]) {
				if got == "" {
					t.Errorf("%s is empty — a blank line of copy reaches the reader as nothing at all", name)
					continue
				}
				if got == english[name] {
					t.Errorf("%s is the English string verbatim; the entry exists and the reader still "+
						"gets English", name)
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
	sentences := langcopytest.Sentences(t, onboardingCopyByLang[textlang.German])
	fields := make([]germanField, 0, len(sentences))
	for name, text := range sentences {
		fields = append(fields, germanField{name, text})
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

func TestTheGermanOnboardingFollowsTheGermanStylePage(t *testing.T) {
	for _, field := range germanOnboarding(t) {
		langcopytest.GermanText(t, field.name, field.text)
	}
}

func TestTheLanguageKeyedCopyFollowsTheStylePages(t *testing.T) {
	langcopytest.StyleByLanguage(t, signalSummaryByLang)
	langcopytest.StyleByLanguage(t, technicalSummaryByLang)
	langcopytest.StyleByLanguage(t, projectHealthByLang)
	langcopytest.StyleByLanguage(t, importProblemsHeaders)
	langcopytest.StyleByLanguage(t, onboardingCopyByLang, langcopytest.Agent("the onboarding conversation is an agent speaking in a chat bubble, which may say I"))
}
