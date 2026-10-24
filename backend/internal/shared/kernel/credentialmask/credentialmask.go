// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package credentialmask is the one rule set that finds passwords, access keys
// and tokens in text. Captured text is masked with it before it is stored, and
// every model-bound payload is stripped with it before it leaves.
//
// It is not a PII filter: names, addresses and phone numbers pass untouched.
package credentialmask

import (
	"bytes"
	"regexp"
)

// Lane is the form of the text a scan reads.
type Lane int

const (
	// Text is text as it is stored, where a line break is a real one.
	Text Lane = iota
	// JSON is text inside an encoded JSON document. A line break there is the
	// escape \n, so the rules that read line structure do not run. No JSON rule
	// matches across a quote or a backslash, so a replacement keeps the framing.
	JSON
)

// Marker is what MaskText puts in place of a credential's value.
const Marker = "[masked]"

// MaskText masks every credential in stored text with Marker and keeps the
// label in front of it. Masked text masks to itself.
func MaskText(s string) string {
	masked, _ := Replace([]byte(s), Text, func(string) string { return Marker })
	return string(masked)
}

// Replace puts mark(kind) in place of the value of every credential the rules
// for lane find in text. It returns the kind of each one it replaced. The
// label before the value and the structure after it stay. A value that already
// is its mark is left alone and not counted, so a second pass finds nothing.
func Replace(text []byte, lane Lane, mark func(kind string) string) ([]byte, []string) {
	var found []string
	for _, r := range rules {
		if !r.runsIn(lane) {
			continue
		}
		text = r.re.ReplaceAllFunc(text, func(match []byte) []byte {
			groups := r.re.FindSubmatch(match)
			value := groups[0]
			if r.keepPrefix {
				value = groups[2]
			}
			marker := []byte(mark(r.kind))
			if bytes.Equal(value, marker) || (r.accept != nil && !r.accept(value)) {
				return match
			}
			found = append(found, r.kind)
			var out []byte
			if r.keepPrefix {
				out = append(out, groups[1]...)
			}
			out = append(out, marker...)
			if r.keepSuffix {
				out = append(out, groups[len(groups)-1]...)
			}
			return out
		})
	}
	return text, found
}

// laneScope is the lane a rule runs in.
type laneScope int

const (
	bothLanes laneScope = iota
	textOnly
	jsonOnly
)

type rule struct {
	kind string
	re   *regexp.Regexp
	// keepPrefix keeps the first group, the label in front of the value.
	keepPrefix bool
	// keepSuffix keeps the last group. RE2 has no lookahead, so a rule that
	// needs a trailing anchor consumes it and puts it back.
	keepSuffix bool
	lane       laneScope
	// accept, when set, must hold for the value or the match stays.
	accept func(value []byte) bool
}

func (r rule) runsIn(lane Lane) bool {
	return r.lane == bothLanes || (r.lane == textOnly) == (lane == Text)
}

// Text labels are of three sorts. A password label is followed by a secret
// whatever its value looks like, and so is a strong label, which names a kind
// of key. A bare ambiguous word also heads prose ("The secret: preparation"),
// so its value must look like a credential.
const (
	passwordLabels  = `password|passwd|pwd|passwort|kennwort|passcode|pin(?:[ -]?code)?`
	strongLabels    = `api[ _-]?key|apikey|access[ _-]?token|auth[ _-]?token|client[ _-]?secret|private[ _-]?key|secret[ _-]?key`
	ambiguousLabels = `secret|token|key`
	loginLabels     = `login|user(?:name)?|benutzer(?:name)?`
	// jsonLabels are the labels of the model-bound payload's assignment rule.
	// pin, token and key are left out: they are common JSON keys with short
	// plain values, such as "pin": true.
	jsonLabels = `password|passwd|pwd|passwort|kennwort|passcode|secret|api[_-]?key|apikey|access[_-]?token|auth[_-]?token|client[_-]?secret|private[_-]?key`
)

// labelled is a label, an optional closing quote and a separator on one line.
func labelled(labels string) string {
	return `(?i)\b((?:` + labels + `)["']?[ \t]*[:=][ \t]*`
}

// looksLikeCredential is one word of at least 8 characters with a digit or a
// symbol in it, which a word of prose almost never is.
func looksLikeCredential(value []byte) bool {
	return len(value) >= 8 && bytes.ContainsFunc(value, func(r rune) bool {
		return !('a' <= r && r <= 'z' || 'A' <= r && r <= 'Z')
	})
}

// toLineEnd is a label, an opening quote if any, and a value that runs to the
// end of the line. A value that excludes the quote never reaches the end of a
// line on which the quote closes.
func toLineEnd(labels, quote, value string) *regexp.Regexp {
	return regexp.MustCompile(`(?m)` + labelled(labels) + quote + `)(` + value + `)([ \t]*\r?)$`)
}

// nextLine is a label alone on its line and a one-word value alone on the next,
// so a heading over a sentence keeps the sentence.
func nextLine(labels string) *regexp.Regexp {
	return regexp.MustCompile(`(?im)^([ \t]*(?:` + labels + `)[ \t]*:?[ \t]*\r?\n[ \t]*)([^\s]{4,})([ \t]*\r?)$`)
}

var rules = []rule{
	// (?s) spans lines; in JSON the line breaks are \n escapes, which .*? crosses too.
	{kind: "private_key", re: regexp.MustCompile(`(?s)-----BEGIN [A-Z ]*PRIVATE KEY-----.*?-----END [A-Z ]*PRIVATE KEY-----`)},
	{kind: "api_key", re: regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{16,}`)},
	// Stripe's underscore keys, a shape the hyphen anchor of sk- never reaches.
	{kind: "api_key", re: regexp.MustCompile(`\b[srp]k_(?:live|test)_[0-9A-Za-z]{16,}`)},
	{kind: "api_key", re: regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9]{20,}`)},
	{kind: "api_key", re: regexp.MustCompile(`\bxox[baprs]-[A-Za-z0-9-]{10,}`)},
	{kind: "api_key", re: regexp.MustCompile(`\bAIza[0-9A-Za-z_-]{30,}`)},
	{kind: "aws_access_key", re: regexp.MustCompile(`\b(?:AKIA|ASIA)[0-9A-Z]{16}\b`)},
	// The 40-character AWS secret has no prefix of its own. Its key name is one
	// underscore-joined word, so the assignment rule's \b never fires in it.
	{
		kind: "aws_secret_key", keepPrefix: true,
		re: regexp.MustCompile(`(?i)\b((?:aws[_-]?)?secret[_-]?access[_-]?key["']?\s*[:=]\s*["']?)([A-Za-z0-9/+]{40})`),
	},
	// scheme://user:PASSWORD@host. The closing @ is kept, so host:port after it
	// is never read as user:password.
	{
		kind: "url_credential", keepPrefix: true, keepSuffix: true,
		re: regexp.MustCompile(`(://[^/\s:@"'\\]+:)([^/\s@"'\\]{2,})(@)`),
	},
	// Before the bearer rule, so the kind names what was caught.
	{kind: "jwt", re: regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}`)},
	{kind: "bearer_token", re: regexp.MustCompile(`(?i)\bbearer[ \t]+[A-Za-z0-9._~+/=-]{16,}`)},
	// The value stops at whitespace, quotes and separators, so only the secret goes.
	{
		kind: "credential_assignment", keepPrefix: true, lane: jsonOnly,
		re: regexp.MustCompile(`(?i)\b((?:` + jsonLabels + `)["']?\s*[:=]\s*["']?)([^\s"'\\,;&]{4,})`),
	},
	// A quoted passphrase is masked through its closing quote. An unquoted one, or
	// one whose quote never closes on its line, is masked to the end of the line.
	{kind: "credential_assignment", keepPrefix: true, keepSuffix: true, lane: textOnly, re: regexp.MustCompile(labelled(passwordLabels) + `")([^"\r\n]+)(")`)},
	{kind: "credential_assignment", keepPrefix: true, keepSuffix: true, lane: textOnly, re: regexp.MustCompile(labelled(passwordLabels) + `')([^'\r\n]+)(')`)},
	{kind: "credential_assignment", keepPrefix: true, keepSuffix: true, lane: textOnly, re: toLineEnd(passwordLabels, `"`, `[^"\r\n]+?`)},
	{kind: "credential_assignment", keepPrefix: true, keepSuffix: true, lane: textOnly, re: toLineEnd(passwordLabels, `'`, `[^'\r\n]+?`)},
	{kind: "credential_assignment", keepPrefix: true, keepSuffix: true, lane: textOnly, re: toLineEnd(passwordLabels, ``, `[^\s"'][^\r\n]*?`)},
	{
		kind: "credential_assignment", keepPrefix: true, lane: textOnly,
		re: regexp.MustCompile(labelled(strongLabels) + `["']?)([^\s"'\\,;&]{4,})`),
	},
	{
		kind: "credential_assignment", keepPrefix: true, lane: textOnly, accept: looksLikeCredential,
		re: regexp.MustCompile(labelled(ambiguousLabels) + `["']?)([^\s"'\\,;&]+)`),
	},
	{kind: "credential_next_line", keepPrefix: true, keepSuffix: true, lane: textOnly, re: nextLine(passwordLabels + `|` + strongLabels)},
	{kind: "credential_next_line", keepPrefix: true, keepSuffix: true, lane: textOnly, accept: looksLikeCredential, re: nextLine(ambiguousLabels)},
	// The login on the line above a password line: the pair is the credential.
	{
		kind: "login_name", keepPrefix: true, keepSuffix: true, lane: textOnly,
		re: regexp.MustCompile(`(?im)^([ \t]*(?:` + loginLabels + `)[ \t]*[:=][ \t]*)([^\s]{2,})([ \t]*\r?\n[ \t]*(?:` + passwordLabels + `)\b)`),
	},
}
