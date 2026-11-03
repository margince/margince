// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package salutation reads the greeting a message opens with and the closing
// lines it ends on. The draft checks and the request reader both ask it, so a
// greeting is one thing to every reader.
package salutation

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/cases"
	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"

	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// greetingLangs are the languages with openers, in the order they are tried.
var greetingLangs = []textlang.Lang{textlang.English, textlang.German, textlang.Vietnamese}

// Langs are the languages a greeting is read in.
func Langs() []textlang.Lang { return append([]textlang.Lang(nil), greetingLangs...) }

// GreetsIn reports whether lang has openers of its own to compare against.
func GreetsIn(lang textlang.Lang) bool {
	_, known := greetingOpeners[lang]
	return known
}

// WordBoundary reports whether the byte at i ends a word, or is off either end.
func WordBoundary(text string, i int) bool {
	if i < 0 || i >= len(text) {
		return true
	}
	// The whole character at i, which may start before i when i is the last
	// byte of the character before a word.
	r, _ := utf8.DecodeRuneInString(text[i:])
	if !utf8.RuneStart(text[i]) {
		r, _ = utf8.DecodeLastRuneInString(text[:i+1])
	}
	return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '\''
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
	"and", "or", "herr", "frau", "hr", "fr", "damen", "herren", "und", "zusammen", "alle",
	"allerseits", "liebes", "kollegen", "kolleginnen",
	"anh", "chị", "em", "ông", "bà", "cô", "chú", "bạn", "quý", "vị", "và", "mọi", "người",
)

// messageWords start the message, never a greeting: a pronoun addresses
// nobody. "Hallo Herr Müller Ihr Angebot" greets Müller, and "Ihr" is the
// capitalised first word of the message.
var messageWords = foldedSet("i", "we", "ich", "wir", "ihr", "ihnen", "sie", "du")

// Greeting is the greeting a text opens with.
type Greeting struct {
	// Langs are the languages the opener greets in.
	Langs []textlang.Lang
	// names are the greeted words that are not an address form, folded.
	Names []string
	// end is the byte offset in the trimmed body where the greeting stops.
	End int
	// closed: punctuation or the line's end bounds the greeting, so end is
	// where a repair may cut it.
	Closed bool
}

// Opening reads the greeting a trimmed, NFC text opens with: an
// opener, then whatever it addresses up to the punctuation that closes it.
//
// A greeting run on into the message with no punctuation ("Hi Marcus I wanted
// to") is neither judged nor cut. Where
// its name ends is a guess, and a wrong cut would eat the message's first words.
func Opening(trimmed string) (Greeting, bool) {
	firstLine, _, _ := strings.Cut(trimmed, "\n")
	firstLine = strings.TrimRight(firstLine, "\r")
	found := Greeting{}
	for _, lang := range greetingLangs {
		for _, opener := range greetingOpeners[lang] {
			end, ok := foldedPrefix(firstLine, opener)
			if !ok || !WordBoundary(firstLine, end) {
				continue
			}
			found.Langs = append(found.Langs, lang)
			found.End = max(found.End, end)
		}
	}
	if len(found.Langs) == 0 {
		return Greeting{}, false
	}
	if names, end, closed := addressee(firstLine, found.End); closed {
		found.Names, found.End, found.Closed = names, end, true
	}
	return found, true
}

// foldedPrefix reports whether line opens with opener, ignoring case, and the
// byte offset in line where it stops. It counts runes, not bytes: "GRÜẞ" is a
// byte longer than "grüß".
func foldedPrefix(line, opener string) (int, bool) {
	end := 0
	for range utf8.RuneCountInString(opener) {
		if end == len(line) {
			return 0, false
		}
		_, width := utf8.DecodeRuneInString(line[end:])
		end += width
	}
	return end, strings.EqualFold(line[:end], opener)
}

// addressee reads the words a greeting addresses from the opener's end. It
// returns the names among them and where the greeting stops. The flag is false
// when a word of the message stopped it before any punctuation did.
//
// A dotted word is an abbreviation ("Hr.", "J.", "Dipl.-Ing.") unless it ends
// the line, so its full stop does not close the greeting.
func addressee(line string, from int) ([]string, int, bool) {
	var names []string
	end, separated := openingSeparator(line, from)
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
		if stopsGreeting(word, named) {
			if separated >= 0 && len(names) == 0 {
				return names, separated, true
			}
			return names, end, false
		}
		end += start + width
		if keepsGreetingOpen(word, field, last) {
			continue
		}
		names = append(names, named...)
		if word != field {
			return names, end, true
		}
	}
	return names, end, true
}

// openingSeparator steps past the separator "Hello, Bob," and "Kính gửi: Anh
// Nam," put between the opener and the name. separated is where a message that
// follows it closes the greeting, or -1 with no separator.
func openingSeparator(line string, from int) (end, separated int) {
	rest := strings.TrimLeft(line[from:], " \t")
	if strings.HasPrefix(rest, ",") || strings.HasPrefix(rest, ":") {
		end = len(line) - len(rest) + 1
		return end, end
	}
	return from, -1
}

// stopsGreeting reports a word that belongs to the message, not the greeting:
// none at all, a word messages open with, or a lower-case name.
func stopsGreeting(word string, named []string) bool {
	initial, _ := utf8.DecodeRuneInString(word)
	return word == "" || messageWords[FoldName(word)] || (len(named) > 0 && !unicode.IsUpper(initial))
}

// keepsGreetingOpen reports a dotted word the greeting reads past. That is one
// with a dot inside it, or an abbreviation whose full stop does not end the line.
func keepsGreetingOpen(word, field string, last bool) bool {
	return strings.Contains(word, ".") || (strings.HasSuffix(field, ".") && !last && abbreviated(word))
}

// abbreviated reports a dotted word that does not end the greeting: an initial
// ("J.") or a title ("Dr.", "Hr."). "Hi Bob. How are you" closes after Bob.
func abbreviated(word string) bool {
	return utf8.RuneCountInString(word) == 1 || addressForms[FoldName(word)]
}

// nameWords are the parts of a greeted word that are not an address form,
// folded. "Sir/Madam" is two address forms and names nobody.
func nameWords(word string) []string {
	var names []string
	for part := range strings.SplitSeq(FoldName(word), "/") {
		if part != "" && !addressForms[part] {
			names = append(names, part)
		}
	}
	return names
}

// FoldName is how a greeted word and a recorded name are compared: no case,
// ß as ss, and no accents. A model often drops one ("Muller" for Müller).
//
// Looser than the contacts module's duplicate fold, which keeps the marks that
// are letters of their own. A greeting is only asked whether it could be this
// recipient, and a false match costs less than replacing a right greeting.
func FoldName(word string) string {
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
		set[FoldName(word)] = true
	}
	return set
}
