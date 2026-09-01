// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// Turning a model's answer back into the plain text the contract promises.
//
// A draft body is plain text end to end — there is no rich-text storage
// format and no HTML+text send pair — but a model asked for prose returns
// what prose looks like on the web, and `<br>` between paragraphs is the
// common shape of that. Nothing downstream converts it: the tag reaches the
// textarea, the send, and the recipient's inbox as four visible characters.
//
// So the break is honoured rather than deleted. A model that wrote `<br><br>`
// meant a paragraph, and dropping the tag would run two paragraphs together —
// which is the same defect wearing a tidier face.

import (
	"regexp"
	"strings"
)

// breakTag matches the line-break tags a model writes, in the spellings it
// writes them: with or without the XHTML slash, with or without inner spaces.
var breakTag = regexp.MustCompile(`(?i)<\s*br\s*/?\s*>`)

// blockClose matches the end of a paragraph-shaped element. It ends a
// PARAGRAPH rather than a line, so it is worth a blank line where a break tag
// is worth one newline — `<p>a</p><p>b</p>` means the same as `a<br><br>b`.
var blockClose = regexp.MustCompile(`(?i)<\s*/\s*(p|div|li|h[1-6])\s*>`)

// blockOpen matches the start of one. It carries no break of its own — the
// preceding close already supplied it — so it simply leaves.
var blockOpen = regexp.MustCompile(`(?i)<\s*(p|div|ul|ol|li|h[1-6])(\s[^<>]*)?/?\s*>`)

// runsOfBlankLines collapses three or more newlines to a paragraph break.
var runsOfBlankLines = regexp.MustCompile(`\n{3,}`)

// PlainText converts a model's answer to the plain text a draft body is
// contractually made of, preserving the structure the model intended.
//
// Only the break-shaped tags are translated. Anything else angle-bracketed is
// left exactly as written: a body may legitimately contain "<" — a comparison,
// an address in angle brackets, a quoted snippet of somebody's own message —
// and a general tag stripper would eat the customer's text to fix our own.
func PlainText(text string) string {
	if !strings.Contains(text, "<") {
		return text
	}
	out := breakTag.ReplaceAllString(text, "\n")
	out = blockClose.ReplaceAllString(out, "\n\n")
	out = blockOpen.ReplaceAllString(out, "")
	// A tag sat on its own line leaves the surrounding newlines behind, so the
	// collapse runs after the replacements rather than as part of them.
	out = runsOfBlankLines.ReplaceAllString(out, "\n\n")
	return strings.TrimSpace(out)
}
