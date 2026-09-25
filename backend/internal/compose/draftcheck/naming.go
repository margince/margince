// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package draftcheck

// Naming a past exchange is not gesturing at one.
//
// After a long gap the prompt asks the draft to say that time has passed and
// name what the exchange was about. "It has been months since we last spoke
// about the integration timeline" does exactly that, and it carries "we last
// spoke" — so the assumed-memory rule reads what FOLLOWS the phrase before it
// refuses it. "As we discussed", "since we last spoke." and "we discussed it"
// name nothing and are still refused.

import (
	"slices"
	"strings"
	"unicode"

	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// namingLeads are the assumed-memory phrases that may open a clause naming the
// exchange, each with the words that must come between it and the subject. An
// empty list means the subject follows directly: "we discussed the rollout".
var namingLeads = map[textlang.Lang]map[string][]string{
	textlang.English: {
		"we discussed":       nil,
		"we spoke about":     nil,
		"we last spoke":      {"about", "regarding", "on"},
		"when we last spoke": {"about", "regarding", "on"},
	},
	textlang.German: {
		"als wir zuletzt": {"über"},
	},
}

// vagueSubjects are the words that fill a subject's place without naming one.
var vagueSubjects = map[string]bool{
	"it": true, "this": true, "that": true, "these": true, "those": true,
	"things": true, "everything": true, "earlier": true, "before": true,
	"previously": true, "last": true, "then": true, "in": true, "at": true,
	"and": true, "but": true, "with": true, "when": true, "so": true, "as": true,
	"es": true, "das": true, "dies": true, "darüber": true,
}

// determiners come before a subject without being one, so the word after them
// is the one tested: "the last time" and "a couple of things" name nothing.
var determiners = map[string]bool{
	"the": true, "a": true, "an": true, "our": true, "your": true,
	"this": true, "these": true, "those": true, "few": true, "couple": true, "of": true,
	"der": true, "die": true, "den": true, "dem": true, "das": true,
	"ein": true, "eine": true, "einen": true, "einem": true, "unser": true, "unsere": true, "unseren": true,
}

// gestures reports whether text carries phrase anywhere as a bare gesture
// rather than as the lead of a clause naming what the exchange was about.
func gestures(lowered, phrase string, lang textlang.Lang) bool {
	connectors, leads := namingLeads[lang][phrase]
	for offset := 0; offset < len(lowered); {
		i := wordIndex(lowered[offset:], phrase)
		if i < 0 {
			return false
		}
		at := offset + i
		if !leads || !namesSubject(lowered[:at], lowered[at+len(phrase):], connectors) {
			return true
		}
		offset = at + len(phrase)
	}
	return false
}

// namesSubject reports whether the words after a naming lead are a subject:
// the connectors in order, then a word that is not a placeholder. Punctuation
// straight after the lead ends the clause, and "as" before it makes the whole
// phrase the gesture.
func namesSubject(before, after string, connectors []string) bool {
	if fields := strings.Fields(before); len(fields) > 0 && fields[len(fields)-1] == "as" {
		return false
	}
	if !strings.HasPrefix(after, " ") {
		return false
	}
	words := strings.Fields(after)
	if len(connectors) > 0 {
		if len(words) == 0 || !slices.Contains(connectors, words[0]) {
			return false
		}
		words = words[1:]
	}
	// A determiner with punctuation on it ends the clause, so it is the subject.
	for len(words) > 1 && determiners[words[0]] {
		words = words[1:]
	}
	if len(words) == 0 {
		return false
	}
	subject := strings.TrimFunc(words[0], func(r rune) bool { return !unicode.IsLetter(r) })
	return subject != "" && !vagueSubjects[subject] && !determiners[subject]
}
