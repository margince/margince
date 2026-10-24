// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package credentialmask is the one rule set that finds passwords, access keys
// and tokens in text. Captured text is masked with it before it is stored, and
// every model-bound payload is stripped with it before it leaves.
//
// It is not a PII filter: names, addresses and phone numbers pass untouched.
package credentialmask

import "regexp"

// Lane is the form of the text a scan reads.
type Lane int

const (
	// Text is text as it is stored, where a line break is a real one.
	Text Lane = iota
	// JSON is text inside an encoded JSON document. A line break there is the
	// escape \n, so the rules that read line structure do not run. No rule
	// matches across a quote or a backslash, so a replacement keeps the framing.
	JSON
)

// Marker is what MaskText puts in place of a credential's value.
const Marker = "[masked]"

// MaskText masks every credential in stored text with Marker and keeps the
// label in front of it. No value class admits "[", so no rule finds a masked
// value again and masked text masks to itself.
func MaskText(s string) string {
	return string(Replace([]byte(s), Text, func(string) []byte { return []byte(Marker) }))
}

// Replace puts marker(kind) in place of the value of every credential the rules
// for lane find in text. The label before the value and the structure after it
// stay.
func Replace(text []byte, lane Lane, marker func(kind string) []byte) []byte {
	for _, r := range rules {
		if r.textOnly && lane != Text {
			continue
		}
		text = r.re.ReplaceAllFunc(text, func(match []byte) []byte {
			if !r.keepPrefix && !r.keepSuffix {
				return marker(r.kind)
			}
			groups := r.re.FindSubmatch(match)
			var out []byte
			if r.keepPrefix {
				out = append(out, groups[1]...)
			}
			out = append(out, marker(r.kind)...)
			if r.keepSuffix {
				out = append(out, groups[len(groups)-1]...)
			}
			return out
		})
	}
	return text
}

type rule struct {
	kind string
	re   *regexp.Regexp
	// keepPrefix keeps the first group, the label in front of the value.
	keepPrefix bool
	// keepSuffix keeps the last group. RE2 has no lookahead, so a rule that
	// needs a trailing anchor consumes it and puts it back.
	keepSuffix bool
	// textOnly marks a rule that reads real line breaks (see JSON).
	textOnly bool
}

// The labels a credential value follows, in both lanes. pin and token are
// common JSON keys with short plain values, so they are named for text only.
const (
	secretLabels   = `password|passwd|pwd|passwort|kennwort|passcode|secret|api[_-]?key|apikey|access[_-]?token|auth[_-]?token|client[_-]?secret|private[_-]?key`
	textLabels     = `pin(?:[ -]?code)?|token`
	passwordLabels = `password|passwd|pwd|passwort|kennwort|passcode|pin`
	loginLabels    = `login|user(?:name)?|benutzer(?:name)?`
)

// assignment matches "label: value" and "label=value" on one line. The value
// stops at whitespace, quotes and separators, so only the secret goes.
func assignment(labels string) *regexp.Regexp {
	return regexp.MustCompile(`(?i)\b((?:` + labels + `)["']?[ \t]*[:=][ \t]*["']?)([^\s"'\\,;&\[]{4,})`)
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
		re: regexp.MustCompile(`(://[^/\s:@"'\\]+:)([^/\s@"'\\\[]{2,})(@)`),
	},
	// Before the bearer rule, so the kind names what was caught.
	{kind: "jwt", re: regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}`)},
	{kind: "bearer_token", re: regexp.MustCompile(`(?i)\bbearer[ \t]+[A-Za-z0-9._~+/=-]{16,}`)},
	{kind: "credential_assignment", keepPrefix: true, re: assignment(secretLabels)},
	{kind: "credential_assignment", keepPrefix: true, textOnly: true, re: assignment(textLabels)},
	// A label alone on its line and the value alone on the next. The value line
	// must be one word, so a heading over a sentence keeps the sentence.
	{
		kind: "credential_next_line", keepPrefix: true, keepSuffix: true, textOnly: true,
		re: regexp.MustCompile(`(?im)^([ \t]*(?:` + secretLabels + `|` + textLabels + `)[ \t]*:?[ \t]*\r?\n[ \t]*)([^\s\[]{4,})([ \t]*\r?)$`),
	},
	// The login on the line above a password line: the pair is the credential.
	{
		kind: "login_name", keepPrefix: true, keepSuffix: true, textOnly: true,
		re: regexp.MustCompile(`(?im)^([ \t]*(?:` + loginLabels + `)[ \t]*[:=][ \t]*)([^\s\[]{2,})([ \t]*\r?\n[ \t]*(?:` + passwordLabels + `)\b)`),
	},
}
