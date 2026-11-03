// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package salutation

import (
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

// Strip is text without its opening greeting or the sign-off lines it ends on.
// A greeting run on into the message keeps its line. Where it stops is a guess,
// and a wrong cut would drop the sender's first words.
func Strip(text string) string {
	trimmed := norm.NFC.String(strings.TrimSpace(text))
	if greeting, ok := Opening(trimmed); ok && greeting.Closed {
		trimmed = strings.TrimSpace(trimmed[greeting.End:])
	}
	lines := strings.Split(trimmed, "\n")
	for len(lines) > 0 {
		last := len(lines) - 1
		switch {
		case strings.TrimSpace(lines[last]) == "" || isClosing(lines[last]):
			lines = lines[:last]
		case last > 0 && signedBy(lineWords(lines[last])) && isClosing(lines[last-1]):
			// The name on a line of its own, under the closing it signs.
			lines = lines[:last-1]
		default:
			return strings.Join(lines, "\n")
		}
	}
	return ""
}

// lineWords are the letters-and-digits runs of a line.
func lineWords(line string) []string {
	return strings.FieldsFunc(line, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
}
