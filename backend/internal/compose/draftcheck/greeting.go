// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package draftcheck

import (
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/cases"
	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"

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
	opening, ok := openingGreeting(trimmed)
	if !ok || !opening.closed || (!opening.misaddressed(to) && !opening.foreign(envelope.Lang())) {
		return body
	}
	if message := strings.TrimSpace(trimmed[opening.end:]); message != "" {
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
	opening, ok := openingGreeting(norm.NFC.String(strings.TrimSpace(body)))
	if !ok || !opening.closed {
		return nil
	}
	var findings []Finding
	if opening.misaddressed(on) {
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
	if opening.foreign(lang) {
		findings = append(findings, Finding{
			Rule:   RuleGreetingLanguage,
			Phrase: "a greeting in another language than the message",
			Why:    "the greeting is in another language than the message — greet in the message's language",
		})
	}
	return findings
}

// greetingOpeners are the words a greeting starts with, by the language they
// greet in. German and Vietnamese business mail borrow the English openers
// listed under them, so a draft opening on one is still in that language.
var greetingOpeners = map[textlang.Lang][]string{
	textlang.English: {
		"hi", "hello", "hey", "dear", "good morning", "good afternoon", "good evening", "good day",
	},
	textlang.German: {
		"hallo", "hi", "hey", "guten tag", "guten morgen", "guten abend", "sehr geehrte",
		"sehr geehrter", "liebe", "lieber", "moin moin", "moin", "servus", "grüß gott", "grüss gott",
		"grüß dich", "grüss dich", "grüezi",
	},
	textlang.Vietnamese: {"xin chào", "chào", "kính gửi", "kính thưa", "thân gửi", "hi", "hello", "dear"},
}

// addressForms are the words a greeting carries around a name without being
// one. They are titles, honorifics, group words and conjunctions, so "Sehr
// geehrte Damen und Herren" names nobody. Keys are folded as a greeted word is.
var addressForms = foldedSet(
	"mr", "mrs", "ms", "mx", "miss", "dr", "prof", "professor", "sir", "sirs", "madam",
	"all", "team", "everyone", "everybody", "there", "folks", "colleagues", "customer",
	"and", "or", "herr", "frau", "damen", "herren", "und", "zusammen", "alle",
	"allerseits", "liebes", "kollegen", "kolleginnen",
	"anh", "chị", "em", "ông", "bà", "cô", "chú", "bạn", "quý", "vị", "và", "mọi", "người",
)

// messageWords start the message, never a greeting: a pronoun addresses
// nobody. "Hallo Herr Müller Ihr Angebot" greets Müller, and "Ihr" is the
// capitalised first word of the message.
var messageWords = foldedSet("i", "we", "ich", "wir", "ihr", "ihnen", "sie", "du")

// nicknameMinRunes is the shortest greeted word that may stand for a longer
// recorded name it begins: "Alex" for Alexander, "Anne" for Anne-Marie. Two
// letters would let "Al" pass for Alice.
const nicknameMinRunes = 3

// greeting is the opening greeting of a draft, as the checks read it.
type greeting struct {
	// langs are the languages the opener greets in.
	langs []textlang.Lang
	// names are the greeted words that are not an address form, folded.
	names []string
	// end is the byte offset in the trimmed body where the greeting stops.
	end int
	// closed: punctuation or the line's end bounds the greeting, so end is
	// where a repair may cut it.
	closed bool
}

// openingGreeting reads the greeting a trimmed, NFC body opens with: an
// opener, then whatever it addresses up to the punctuation that closes it.
//
// A greeting run on into the message with no punctuation ("Hi Marcus I wanted
// to") is neither judged nor cut. Where
// its name ends is a guess, and a wrong cut would eat the message's first words.
func openingGreeting(trimmed string) (greeting, bool) {
	firstLine, _, _ := strings.Cut(trimmed, "\n")
	firstLine = strings.TrimRight(firstLine, "\r")
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
	if names, end, closed := addressee(firstLine, found.end); closed {
		found.names, found.end, found.closed = names, end, true
	}
	return found, true
}

// addressee reads the words a greeting addresses from the opener's end. It
// returns the names among them and where the greeting stops. The flag is false
// when a word of the message stopped it before any punctuation did.
//
// A dotted word is an abbreviation ("Hr.", "J.", "Dipl.-Ing.") unless it ends
// the line, so its full stop does not close the greeting.
func addressee(line string, from int) ([]string, int, bool) {
	var names []string
	end := from
	// "Hello, Bob," puts a comma between the opener and the name.
	if rest := strings.TrimLeft(line[end:], " \t"); strings.HasPrefix(rest, ",") {
		end = len(line) - len(rest) + 1
	}
	for end < len(line) {
		rest := line[end:]
		start := len(rest) - len(strings.TrimLeft(rest, " \t"))
		if start == len(rest) {
			break
		}
		if strings.ContainsRune(",!.:;", rune(rest[start])) {
			return names, end + start + 1, true
		}
		width := strings.IndexAny(rest[start:], " \t")
		last := width < 0
		if last {
			width = len(rest) - start
		}
		field := rest[start : start+width]
		word := strings.TrimRightFunc(field, unicode.IsPunct)
		named := nameWords(word)
		initial, _ := utf8.DecodeRuneInString(word)
		if word == "" || messageWords[foldName(word)] || (len(named) > 0 && !unicode.IsUpper(initial)) {
			return names, end, false
		}
		end += start + width
		if strings.Contains(word, ".") || (strings.HasSuffix(field, ".") && !last) {
			continue
		}
		names = append(names, named...)
		if word != field {
			return names, end, true
		}
	}
	return names, end, true
}

// nameWords are the parts of a greeted word that are not an address form,
// folded. "Sir/Madam" is two address forms and names nobody.
func nameWords(word string) []string {
	var names []string
	for part := range strings.SplitSeq(foldName(word), "/") {
		if part != "" && !addressForms[part] {
			names = append(names, part)
		}
	}
	return names
}

// misaddressed reports a greeting that names somebody the draft does not go
// to. With no name on record for any reader, every greeted name is one. A
// greeted word passes as a recorded name part, or as a nickname that begins one.
func (g greeting) misaddressed(to Grounds) bool {
	recorded := nameParts(to)
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

// nameParts are the words of every name the draft goes to, and the halves of
// each hyphenated one, folded as a greeted word is.
func nameParts(to Grounds) []string {
	var parts []string
	names := strings.Join(append([]string{to.FirstName, to.LastName}, to.CoRecipients...), " ")
	for word := range strings.FieldsSeq(names) {
		word = foldName(word)
		parts = append(parts, word)
		if halves := strings.Split(word, "-"); len(halves) > 1 {
			parts = append(parts, halves...)
		}
	}
	return parts
}

// foldName is how a greeted word and a recorded name are compared: no case,
// ß as ss, and no accents. A model often drops one ("Muller" for Müller).
//
// Looser than the contacts module's duplicate fold, which keeps the marks that
// are letters of their own. A greeting is only asked whether it could be this
// recipient, and a false match costs less than replacing a right greeting.
func foldName(word string) string {
	stripped, _, err := transform.String(
		transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC), word)
	if err != nil {
		stripped = word
	}
	return cases.Fold().String(stripped)
}

// foldedSet is a lookup keyed by folded words. A list keeps its accents as
// written and still matches a greeted word folded the same way.
func foldedSet(words ...string) map[string]bool {
	set := make(map[string]bool, len(words))
	for _, word := range words {
		set[foldName(word)] = true
	}
	return set
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
