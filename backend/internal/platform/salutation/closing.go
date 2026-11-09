// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package salutation

import (
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"

	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// closings are the lines a message signs off with, folded. A line that is one
// of them, with at most a name after it, says nothing the sender asks for.
var closings = foldedSet(
	"thanks", "thank you", "thanks again", "thank you again", "many thanks", "thanks a lot",
	"best", "best regards", "kind regards", "regards", "warm regards", "cheers", "all the best",
	"sincerely", "yours sincerely", "yours truly", "best wishes", "kind wishes",
	"danke", "vielen dank", "danke schön", "danke schon", "besten dank",
	"mit freundlichen grüßen", "freundliche grüße", "viele grüße", "beste grüße", "liebe grüße", "grüße", "lg", "vg",
	"cảm ơn", "xin cảm ơn", "trân trọng", "thân ái", "thân mến",
)

// closingWordsMax is the longest closing, so a long line is never read whole.
var closingWordsMax = longestClosing()

func longestClosing() int {
	longest := 0
	for closing := range closings {
		longest = max(longest, len(strings.Fields(closing)))
	}
	return longest
}

// nameWordsMax is the longest name a sign-off line may carry after its closing.
const nameWordsMax = 2

// isClosing reports a line that is a sign-off, with at most a name after it.
// The name is capitalised words, so "Thanks, could you send it" is an ask.
func isClosing(line string) bool {
	words := lineWords(line)
	for n := min(len(words), closingWordsMax); n > 0; n-- {
		if closings[FoldName(strings.Join(words[:n], " "))] {
			return signedBy(words[n:])
		}
	}
	return false
}

// signedBy reports words that can be the sender's name: few, each capitalised.
func signedBy(words []string) bool {
	if len(words) > nameWordsMax {
		return false
	}
	for _, word := range words {
		if first, _ := utf8.DecodeRuneInString(word); !unicode.IsUpper(first) {
			return false
		}
	}
	return true
}

// signatureLinesMax bounds the lines a sign-off may carry under its closing: a
// name, a title, a company, a phone number and an address. More is a message.
const signatureLinesMax = 5

// signatureWordsMax bounds one signature line.
const signatureWordsMax = 6

// titleWords are the lower-case words a job title or a company name carries.
var titleWords = foldedSet("of", "and", "for", "the", "für", "und", "von", "der", "des", "và")

// Strip is text without its opening greeting or the sign-off it ends on. It is
// empty for a message that is nothing else. A greeting run on into the message
// keeps its line: where it stops is a guess, and a wrong cut drops the sender's
// first words.
func Strip(text string) string {
	trimmed := norm.NFC.String(strings.TrimSpace(text))
	body := trimmed
	if greeting, ok := Opening(trimmed); ok && greeting.Closed {
		body = strings.TrimSpace(trimmed[greeting.End:])
		if len(greeting.Names) == 0 {
			body = withoutNameLine(body)
		}
	}
	return withoutSignOff(strings.Split(body, "\n"))
}

// QuotedPast reports whether quote appears in text once its greeting and
// sign-off are gone, and says something besides them. The quote is read on its
// own too, because Strip keeps any salutation it cannot place. Strip composes
// text to NFC, so quote is composed as well.
func QuotedPast(text, quote string) bool {
	return Strip(quote) != "" && values.Quoted(Strip(text), norm.NFC.String(quote))
}

// withoutNameLine drops the name a greeting closed before reaching, as in
// "Hi,\nJohn Smith,\nPlease send it."
func withoutNameLine(body string) string {
	first, rest, multiline := strings.Cut(body, "\n")
	line := strings.TrimSpace(first)
	if multiline && strings.HasSuffix(line, ",") && signedBy(lineWords(line)) {
		return strings.TrimSpace(rest)
	}
	return body
}

// withoutSignOff drops the closing a message ends on, with the signature lines
// under it and the blank lines between.
func withoutSignOff(lines []string) string {
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	signature := 0
	for i, raw := range slices.Backward(lines) {
		line := strings.TrimSpace(raw)
		switch {
		case line == "":
			continue
		case isClosing(line):
			return strings.TrimSpace(strings.Join(lines[:i], "\n"))
		case signature < signatureLinesMax && signatureLine(line):
			signature++
		default:
			return strings.TrimSpace(strings.Join(lines, "\n"))
		}
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

// signatureLine reports a line that can stand under a closing: a mail or web
// address, or a few capitalised words that end no sentence. A line it refuses
// stops the walk, so an ask after a mid-message "Thanks!" stays in the message.
func signatureLine(line string) bool {
	if strings.ContainsAny(line, "?!") || strings.HasSuffix(line, ".") {
		return false
	}
	if len(strings.Fields(line)) == 1 && (strings.Contains(line, "@") || strings.HasPrefix(line, "www.")) {
		return true
	}
	words := lineWords(line)
	if len(words) == 0 || len(words) > signatureWordsMax {
		return false
	}
	return !slices.ContainsFunc(words, func(word string) bool {
		first, _ := utf8.DecodeRuneInString(word)
		return !unicode.IsUpper(first) && !unicode.IsDigit(first) && !titleWords[FoldName(word)]
	})
}

// lineWords are the letters-and-digits runs of a line.
func lineWords(line string) []string {
	return strings.FieldsFunc(line, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
}
