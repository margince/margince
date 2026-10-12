// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package draftcheck

import (
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"

	"github.com/margince/margince/backend/internal/shared/kernel/draftfloor"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// EnsureGreeting gives a body the floor's greeting line once the correction
// retry is spent. No draft is served ungreeted, greeting somebody else, or
// greeting in another language. A missing greeting is added above the body,
// and a wrong one is replaced.
func EnsureGreeting(body string, envelope draftfloor.Envelope, firstName, lastName string) string {
	firstName, lastName = draftfloor.NameLine(firstName), draftfloor.NameLine(lastName)
	floor := envelope.Greeting(firstName, lastName)
	trimmed := strings.TrimSpace(body)
	if missingGreeting(trimmed, firstName, lastName) {
		return floor + "\n\n" + trimmed
	}
	opening, ok := openingGreeting(trimmed)
	if !ok || !opening.closed ||
		(!opening.misaddressed(firstName, lastName) && !opening.foreign(envelope.Lang())) {
		return body
	}
	if message := strings.TrimSpace(trimmed[opening.end:]); message != "" {
		return floor + "\n\n" + message
	}
	return floor
}

// greetingFindings judges the greeting a trimmed body opens with: who it
// addresses, and the language it greets in.
//
// The correction names no recorded name. It rides outside the fence, and a
// stored name is text anybody with write access to a contact chose.
func greetingFindings(trimmed string, lang textlang.Lang, on Grounds) []Finding {
	opening, ok := openingGreeting(trimmed)
	if !ok {
		return nil
	}
	var findings []Finding
	if opening.misaddressed(on.FirstName, on.LastName) {
		why := "the greeting names somebody other than the recipient — greet them by the " +
			"name the input gives for the recipient"
		if strings.TrimSpace(on.FirstName+on.LastName) == "" {
			why = "the input gives no name for the recipient, so a name in the greeting is " +
				"invented — open with a greeting that names nobody"
		}
		findings = append(findings, Finding{
			Rule: RuleWrongAddressee, Phrase: trimmed[:opening.end], Why: why,
		})
	}
	if opening.foreign(lang) {
		findings = append(findings, Finding{
			Rule:   RuleGreetingLanguage,
			Phrase: opening.opener,
			Why:    "the greeting is in another language than the message — greet in the message's language",
		})
	}
	return findings
}

// greetingOpeners are the words a greeting starts with, by the language they
// greet in. "Hi" and "Hey" are German too: German correspondence borrows them,
// and a du draft opening "Hi Marcus" is written in German.
var greetingOpeners = map[textlang.Lang][]string{
	textlang.English: {
		"hi", "hello", "hey", "dear", "good morning", "good afternoon", "good evening", "good day",
	},
	textlang.German: {
		"hallo", "hi", "hey", "guten tag", "guten morgen", "guten abend", "sehr geehrte",
		"sehr geehrter", "liebe", "lieber", "moin moin", "moin", "servus", "grüß gott", "grüss gott",
		"grüß dich", "grüss dich", "grüezi",
	},
	textlang.Vietnamese: {"xin chào", "chào", "kính gửi", "kính thưa", "thân gửi"},
}

// addressForms are the words a greeting carries around a name without being
// one. They are titles, honorifics, group words and conjunctions, so "Sehr
// geehrte Damen und Herren" names nobody.
var addressForms = map[string]bool{
	"mr": true, "mrs": true, "ms": true, "miss": true, "dr": true, "prof": true,
	"sir": true, "madam": true, "all": true, "team": true, "everyone": true,
	"everybody": true, "there": true, "folks": true, "and": true, "or": true,
	"herr": true, "frau": true, "damen": true, "herren": true, "und": true,
	"zusammen": true, "alle": true, "allerseits": true, "liebes": true,
	"kollegen": true, "kolleginnen": true,
	"anh": true, "chị": true, "em": true, "ông": true, "bà": true, "cô": true,
	"chú": true, "bạn": true, "quý": true, "và": true, "mọi": true, "người": true,
}

// nicknameMinRunes is the shortest greeted word that may stand for a longer
// recorded name it begins: "Alex" for Alexander, "Anne" for Anne-Marie. Two
// letters would let "Al" pass for Alice.
const nicknameMinRunes = 3

// greeting is the opening greeting of a draft, as the checks read it.
type greeting struct {
	// langs are the languages the opener greets in.
	langs []textlang.Lang
	// opener is the greeting word as the draft wrote it, for a finding to quote.
	opener string
	// names are the greeted words that are not an address form, folded.
	names []string
	// end is the byte offset in the trimmed body where the greeting stops.
	end int
	// closed: punctuation or the line's end bounds the greeting, so end is
	// where a repair may cut it.
	closed bool
}

// openingGreeting reads the greeting a trimmed body opens with: an opener, then
// whatever it addresses up to the punctuation that closes the greeting.
//
// A greeting run on into the message with no punctuation ("Hi Marcus I wanted
// to") is left unjudged. Where its name ends is a guess, and a repair cut at
// the wrong word would eat the message's first words.
func openingGreeting(trimmed string) (greeting, bool) {
	firstLine, _, _ := strings.Cut(trimmed, "\n")
	found := greeting{}
	for _, lang := range []textlang.Lang{textlang.English, textlang.German, textlang.Vietnamese} {
		for _, opener := range greetingOpeners[lang] {
			if len(firstLine) < len(opener) || !strings.EqualFold(firstLine[:len(opener)], opener) ||
				!boundary(firstLine, len(opener)) {
				continue
			}
			found.langs = append(found.langs, lang)
			found.end = max(found.end, len(opener))
		}
	}
	if len(found.langs) == 0 {
		return greeting{}, false
	}
	found.opener = firstLine[:found.end]
	if names, end, closed := addressee(firstLine, found.end); closed {
		found.names, found.end, found.closed = names, end, true
	}
	return found, true
}

// addressee reads the words a greeting addresses from the opener's end. It
// returns the names among them and where the greeting stops. The flag is false
// when a lower-case word of the message stopped it before any punctuation.
func addressee(line string, from int) ([]string, int, bool) {
	var names []string
	end := from
	for end < len(line) {
		rest := line[end:]
		start := len(rest) - len(strings.TrimLeft(rest, " \t"))
		if start < len(rest) && strings.ContainsRune(",!.:;", rune(rest[start])) {
			return names, end + start + 1, true
		}
		width := strings.IndexAny(rest[start:], " \t")
		if width < 0 {
			width = len(rest) - start
		}
		field := rest[start : start+width]
		word := strings.TrimRightFunc(field, unicode.IsPunct)
		key := foldName(word)
		initial, _ := utf8.DecodeRuneInString(word)
		if word == "" || (!addressForms[key] && unicode.IsLower(initial)) {
			return names, end, false
		}
		end += start + width
		if !addressForms[key] {
			names = append(names, key)
		}
		// A title's full stop abbreviates it ("Dr."); any other closes the greeting.
		if word != field && !(addressForms[key] && strings.HasSuffix(field, ".")) {
			return names, end, true
		}
	}
	return names, end, true
}

// misaddressed reports a greeting that names somebody other than the
// recipient, or names anybody at all when the record holds no name for them.
// A greeted word passes as a recorded name part, or as a nickname that begins
// one.
func (g greeting) misaddressed(firstName, lastName string) bool {
	recorded := nameParts(firstName + " " + lastName)
	for _, name := range g.names {
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
func (g greeting) foreign(lang textlang.Lang) bool {
	_, known := greetingOpeners[lang]
	return known && !slices.Contains(g.langs, lang)
}

// nameParts are a recorded name's words, and the halves of each hyphenated
// one, folded as a greeted word is.
func nameParts(name string) []string {
	var parts []string
	for _, word := range strings.Fields(name) {
		word = foldName(word)
		parts = append(parts, word)
		if halves := strings.Split(word, "-"); len(halves) > 1 {
			parts = append(parts, halves...)
		}
	}
	return parts
}

// foldName is how a greeted word and a recorded name are compared: composed
// accents, so "Nguyễn" typed two ways is one name, and no case.
func foldName(word string) string { return strings.ToLower(norm.NFC.String(word)) }

// missingGreeting reports a body whose first line does not greet anybody.
//
// A first line greets when it opens on a greeting word, or on the recipient's
// own name followed by a comma, an exclamation mark or the full stop
// SplitGreetingLine also accepts ("Greven,"). A run-on
// greeting still counts: RuleUnbrokenBlock is the rule for a greeting with no
// line of its own.
func missingGreeting(body, firstName, lastName string) bool {
	first, _, _ := strings.Cut(strings.TrimSpace(body), "\n")
	first = strings.ToLower(strings.TrimSpace(first))
	if first == "" {
		return false
	}
	if _, ok := openingGreeting(first); ok {
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
