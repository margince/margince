// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"regexp"
	"strings"

	"github.com/margince/margince/backend/internal/shared/kernel/mailquote"
)

// Stored mail retains its signature for enrichment; previews remove it for reading.
// The parity gate compares this display policy and shared mailquote patterns with the browser.
// Language detection may discard all text, but a preview must preserve a nonempty body.

// signatureDelimiter is RFC 3676 §4.3: a line of exactly two hyphens.
// Trailing whitespace is allowed because mail clients strip it inconsistently.
var signatureDelimiter = regexp.MustCompile(`^--\s*$`)

// signOffPrefixes may open a line, since a name usually follows on the same one.
var signOffPrefixes = []string{
	"mit freundlichen grüßen",
	"mit freundlichem gruß",
	"mit besten grüßen",
	"freundliche grüße",
	"viele grüße",
	"beste grüße",
	"herzliche grüße",
	"liebe grüße",
	"schöne grüße",
	"best regards",
	"kind regards",
	"warm regards",
	"many thanks",
	"thanks and regards",
	"diese e-mail wurde von avast",
}

// signOffLines is mobile-client boilerplate, matched against the WHOLE line
// rather than as a prefix: "Sent from my perspective, the contract is not
// ready" opens a sentence with the same three words and is the message.
var signOffLines = []*regexp.Regexp{
	regexp.MustCompile(`^von meinem (iphone|ipad|android|mobilteil|samsung)\b.*gesendet$`),
	regexp.MustCompile(`^sent from my \w[\w\s]{0,30}$`),
	regexp.MustCompile(`^gesendet mit \w[\w\s]{0,30}$`),
	regexp.MustCompile(`^get outlook for \w+$`),
}

// signOffExact are the short forms, whole-line only: "LG" opens a sentence
// about a washing machine as readily as it closes a mail.
var signOffExact = map[string]bool{
	"mfg": true, "vg": true, "lg": true, "vlg": true,
	"best": true, "regards": true, "cheers": true, "thanks": true,
	"thank you": true, "danke": true, "gruß": true, "grüße": true, "bg": true,
}

// trailingScanLines is how far up from the end a sign-off is looked for. The
// same words in an opening paragraph are prose, and scanning the whole body
// folds the message away on the strength of one line.
const trailingScanLines = 15

// EmailBodyTail says what the trimmed part of a body starts with.
//
// A signature and a quoted reply are trimmed for different reasons and read
// differently to a contact: the sign-off is the sender still speaking, and the
// quote is an older message. One label for both hid a sender's own name behind
// "show quoted history" on every message that had no history.
type EmailBodyTail string

// The three tails a body can have. Mirrored in frontend/src/format/emailtext.ts
// and held to one table of cases by gates/frontendemailtext_test.go.
const (
	// TailNone: the whole body is the message, with nothing trimmed.
	TailNone EmailBodyTail = "none"
	// TailSignature: a sign-off. The sender still speaking, so the viewer shows
	// it under the message rather than folding it away.
	TailSignature EmailBodyTail = "signature"
	// TailQuote: an older message underneath this one, which the viewer folds.
	TailQuote EmailBodyTail = "quote"
)

// EmailBodyParts is a mail body read for display.
type EmailBodyParts struct {
	// Header is the From:/To: preamble capture folds into the body, when present.
	Header string
	// Main is what the sender actually wrote. Never empty when the body was not.
	Main string
	// Trimmed is the signature and quoted history, kept but not shown.
	Trimmed string
	// Tail is what Trimmed starts with, so a caller can label the control it
	// folds it behind for what is actually under it.
	Tail EmailBodyTail
}

func isSignOff(line string) bool {
	normalized := strings.ToLower(strings.TrimSpace(line))
	normalized = strings.TrimRight(normalized, ",.!")
	if normalized == "" {
		return false
	}
	if signOffExact[normalized] {
		return true
	}
	for _, pattern := range signOffLines {
		if pattern.MatchString(normalized) {
			return true
		}
	}
	for _, prefix := range signOffPrefixes {
		if strings.HasPrefix(normalized, prefix) {
			return true
		}
	}
	return false
}

// boundaryIndex is the first line belonging to the tail rather than the
// message, and which kind of tail it opens. The index is -1 when the whole
// body is the message. A sign-off counts only above the quoted earlier mail.
func boundaryIndex(lines []string, subject string) (int, EmailBodyTail) {
	quote := mailquote.OwnTextEnd(lines, subject)
	signOffFloor := max(0, len(lines)-trailingScanLines)
	for i, line := range lines[:quote] {
		if signatureDelimiter.MatchString(line) || (i >= signOffFloor && isSignOff(line)) {
			return i, TailSignature
		}
	}
	if quote < len(lines) {
		return quote, TailQuote
	}
	return -1, TailNone
}

// SplitEmailBody separates a stored mail body into the part worth reading on a
// row and the tail worth keeping but not showing. The subject is the mail's
// own, which says whether it forwards.
//
// The message is never allowed to become empty: a body that is nothing but a
// greeting is a real message, and a heuristic that folds it away has hidden the
// mail rather than tidied it.
func SplitEmailBody(body, subject string) EmailBodyParts {
	if strings.TrimSpace(body) == "" {
		return EmailBodyParts{Tail: TailNone}
	}
	preamble, rest := mailquote.Preamble(body)
	header := strings.TrimSpace(preamble)

	// A body that is only a preamble has no message under it. The header is the
	// whole of what was captured, so it IS the message: the invariant is that a
	// non-empty body never renders as nothing.
	if strings.TrimSpace(rest) == "" {
		return EmailBodyParts{Main: strings.TrimSpace(body), Tail: TailNone}
	}

	lines := strings.Split(rest, "\n")
	cut, tail := boundaryIndex(lines, subject)
	if cut < 0 {
		return EmailBodyParts{Header: header, Main: strings.TrimSpace(rest), Tail: TailNone}
	}
	main := strings.TrimSpace(strings.Join(lines[:cut], "\n"))
	if main == "" {
		return EmailBodyParts{Header: header, Main: strings.TrimSpace(rest), Tail: TailNone}
	}
	return EmailBodyParts{
		Header:  header,
		Main:    main,
		Trimmed: strings.TrimSpace(strings.Join(lines[cut:], "\n")),
		Tail:    tail,
	}
}

// collapseWhitespace is the one-line form: every run of whitespace becomes a
// single space.
var collapseWhitespace = regexp.MustCompile(`\s+`)

// EmailSummaryText is the message on one line, for a row preview: the sender's
// own words, never their sign-off and never the `From:` preamble.
func EmailSummaryText(body, subject string) string {
	collapsed := strings.TrimSpace(collapseWhitespace.ReplaceAllString(SplitEmailBody(body, subject).Main, " "))
	// A body that is only a quoted forward has no message of its own. The whole
	// text is a better preview than an empty one.
	if collapsed != "" {
		return collapsed
	}
	return strings.TrimSpace(collapseWhitespace.ReplaceAllString(body, " "))
}
