// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package salutation

import (
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
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

// nameWordsMax is the longest name a sign-off line may carry after its closing.
const nameWordsMax = 2

// isClosing reports a line that is a sign-off, with at most a name after it.
// The name is capitalised words, so "Thanks, could you send it" is an ask.
func isClosing(line string) bool {
	words := lineWords(line)
	for n := len(words); n > 0; n-- {
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

// signatureLinesMax bounds the lines a sign-off may carry under its closing:
// a name, a title or a company. More is a message, not a signature.
const signatureLinesMax = 3

// signatureWordsMax bounds one signature line.
const signatureWordsMax = 6

// Strip is text without its opening greeting or the sign-off it ends on. A
// greeting run on into the message keeps its line. Where it stops is a guess,
// and a wrong cut would drop the sender's first words. A message that is all
// greeting and sign-off is kept whole: it asks nothing, and nothing in it is cut.
func Strip(text string) string {
	trimmed := norm.NFC.String(strings.TrimSpace(text))
	body := trimmed
	if greeting, ok := Opening(trimmed); ok && greeting.Closed {
		body = strings.TrimSpace(trimmed[greeting.End:])
	}
	if kept := withoutSignOff(strings.Split(body, "\n")); kept != "" {
		return kept
	}
	return trimmed
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

// signatureLine reports a line that can stand under a closing: a few words,
// capitalised, that end no sentence.
func signatureLine(line string) bool {
	words := lineWords(line)
	return len(words) > 0 && len(words) <= signatureWordsMax &&
		!strings.ContainsAny(line, "?!") && !strings.HasSuffix(line, ".") && signedBy(words[:1])
}

// lineWords are the letters-and-digits runs of a line.
func lineWords(line string) []string {
	return strings.FieldsFunc(line, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
}
