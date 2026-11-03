// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package draftcheck

import (
	"slices"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"

	"github.com/margince/margince/backend/internal/platform/salutation"
	"github.com/margince/margince/backend/internal/shared/kernel/draftfloor"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// EnsureGreeting adds the floor's greeting above a body that opens without one,
// and puts it in place of a closed greeting the checks judge wrong.
func EnsureGreeting(body string, envelope draftfloor.Envelope, to Grounds) string {
	to.FirstName, to.LastName = draftfloor.NameLine(to.FirstName), draftfloor.NameLine(to.LastName)
	floor := envelope.GreetingTo(to.FirstName, to.LastName, len(to.CoRecipients))
	trimmed := norm.NFC.String(strings.TrimSpace(body))
	if missingGreeting(trimmed, to.FirstName, to.LastName) {
		return floor + "\n\n" + trimmed
	}
	opening, ok := salutation.Opening(trimmed)
	if !ok || !opening.Closed || (!misaddressed(opening, to) && !foreign(opening, envelope.Lang())) {
		return body
	}
	if message := strings.TrimSpace(trimmed[opening.End:]); message != "" {
		return floor + "\n\n" + message
	}
	return floor
}

// greetingFindings judges the greeting a body opens with: who it addresses,
// and the language it greets in.
//
// Each finding quotes a fixed description, never the greeting. The correction
// rides outside the fence and the phrase reaches the log, so a name the model
// copied out of the input must reach neither.
func greetingFindings(body string, lang textlang.Lang, on Grounds) []Finding {
	opening, ok := salutation.Opening(norm.NFC.String(strings.TrimSpace(body)))
	if !ok || !opening.Closed {
		return nil
	}
	var findings []Finding
	if misaddressed(opening, on) {
		why := "the greeting names somebody other than the recipient — greet them by the " +
			"name the input gives for the recipient"
		if len(nameParts(on)) == 0 {
			why = "the input gives no name for the recipient, so a name in the greeting is " +
				"invented — open with a greeting that names nobody"
		}
		findings = append(findings, Finding{
			Rule: RuleWrongAddressee, Phrase: "a greeting that names somebody other than the recipient", Why: why,
		})
	}
	if foreign(opening, lang) {
		findings = append(findings, Finding{
			Rule:   RuleGreetingLanguage,
			Phrase: "a greeting in another language than the message",
			Why:    "the greeting is in another language than the message — greet in the message's language",
		})
	}
	return findings
}

// nicknameMinRunes is the shortest greeted word that may stand for a longer
// recorded name it begins: "Alex" for Alexander, "Anne" for Anne-Marie. Two
// letters would let "Al" pass for Alice.
const nicknameMinRunes = 3

// misaddressed reports a greeting that names somebody the draft does not go
// to. With no name on record for any reader, every greeted name is one. A
// greeted word passes as a recorded name part, or as a nickname that begins one.
func misaddressed(g salutation.Greeting, to Grounds) bool {
	recorded := nameParts(to)
	for _, name := range g.Names {
		if !slices.ContainsFunc(recorded, func(part string) bool {
			return part == name ||
				(utf8.RuneCountInString(name) >= nicknameMinRunes && strings.HasPrefix(part, name))
		}) {
			return true
		}
	}
	return false
}

// foreign reports a greeting in another language than the draft. A language
// with no openers of its own has nothing to compare against.
func foreign(g salutation.Greeting, lang textlang.Lang) bool {
	return salutation.GreetsIn(lang) && !slices.Contains(g.Langs, lang)
}

// nameParts are the words of every name the draft goes to, and the halves of
// each hyphenated one, folded as a greeted word is.
func nameParts(to Grounds) []string {
	var parts []string
	names := strings.Join(append([]string{to.FirstName, to.LastName}, to.CoRecipients...), " ")
	for word := range strings.FieldsSeq(names) {
		word = salutation.FoldName(word)
		parts = append(parts, word)
		if halves := strings.Split(word, "-"); len(halves) > 1 {
			parts = append(parts, halves...)
		}
	}
	return parts
}

// missingGreeting reports a body whose first line does not greet anybody.
//
// A first line greets when it opens on a greeting word, or on the recipient's
// own name followed by a comma, an exclamation mark or the full stop
// SplitGreetingLine also accepts ("Greven,"). A run-on
// greeting still counts: RuleUnbrokenBlock is the rule for a greeting with no
// line of its own.
func missingGreeting(body, firstName, lastName string) bool {
	first, _, _ := strings.Cut(norm.NFC.String(strings.TrimSpace(body)), "\n")
	first = strings.ToLower(strings.TrimSpace(first))
	if first == "" {
		return false
	}
	if _, ok := salutation.Opening(first); ok {
		return false
	}
	return !opensOnName(first, firstName, lastName)
}

// opensOnName reports a line that starts with one of the recipient's names,
// whole, and then a comma, an exclamation mark or a full stop.
func opensOnName(line, firstName, lastName string) bool {
	firstName, lastName = draftfloor.NameLine(firstName), draftfloor.NameLine(lastName)
	full := draftfloor.NameLine(firstName + " " + lastName)
	for _, name := range []string{full, firstName, lastName} {
		name = strings.ToLower(strings.TrimSpace(name))
		if name == "" || !strings.HasPrefix(line, name) {
			continue
		}
		if rest := line[len(name):]; rest != "" && strings.ContainsRune(",!.", rune(rest[0])) {
			return true
		}
	}
	return false
}
