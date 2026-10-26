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
	"slices"
	"strings"
)

// Kinds a finding reports; the AI stripper writes them into its marker.
const (
	kindAPIKey               = "api_key"
	kindCredentialAssignment = "credential_assignment" //nolint:gosec // G101: the name of a finding kind, not a credential
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
	return replaceCued(text, lane, mark, true)
}

// replaceCued is Replace with the cue check switchable, so a test can hold the
// checked and the unchecked scan equal.
func replaceCued(text []byte, lane Lane, mark func(kind string) string, cued bool) ([]byte, []string) {
	var found []string
	scan := newCueScan(text)
	for _, r := range rules {
		if !r.runsIn(lane) || (cued && !scan.mayMatch(r.cues)) {
			continue
		}
		replaced := len(found)
		text = r.replaceAll(text, mark, &found)
		// A mark may itself spell a later rule's cue.
		if len(found) > replaced {
			scan = newCueScan(text)
		}
	}
	return text, found
}

// replaceAll reads each match's groups where the match sits in text. Matched
// again on its own, a match can lose the \b its first character had in place,
// and then no group is left to read.
func (r rule) replaceAll(text []byte, mark func(kind string) string, found *[]string) []byte {
	matches := r.re.FindAllSubmatchIndex(text, -1)
	if matches == nil {
		return text
	}
	out := make([]byte, 0, len(text))
	last := 0
	for _, at := range matches {
		out = append(out, text[last:at[0]]...)
		out = append(out, r.replacement(text, at, mark, found)...)
		last = at[1]
	}
	return append(out, text[last:]...)
}

// replacement is what one match becomes: its mark, with the label in front and
// the anchor after it kept where the rule keeps them.
func (r rule) replacement(text []byte, at []int, mark func(kind string) string, found *[]string) []byte {
	group := func(i int) []byte {
		if at[2*i] < 0 {
			return nil
		}
		return text[at[2*i]:at[2*i+1]]
	}
	match, value := group(0), group(0)
	if r.keepPrefix {
		value = group(2)
	}
	marker := []byte(mark(r.kind))
	if bytes.Equal(value, marker) || (r.accept != nil && !r.accept(value)) {
		return match
	}
	*found = append(*found, r.kind)
	var out []byte
	if r.keepPrefix {
		out = append(out, group(1)...)
	}
	out = append(out, marker...)
	if r.keepSuffix {
		out = append(out, group(len(at)/2-1)...)
	}
	return out
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
	// cues are read off re at start-up (cues.go).
	cues [][]byte
	// witnesses are texts the rule must mask (witnesses.go).
	witnesses []string
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
		return (r < 'a' || r > 'z') && (r < 'A' || r > 'Z')
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

var rules = withCues([]rule{
	// (?s) spans lines; in JSON the line breaks are \n escapes, which .*? crosses too.
	{
		kind: "private_key", re: regexp.MustCompile(`(?s)-----BEGIN [A-Z ]*PRIVATE KEY-----.*?-----END [A-Z ]*PRIVATE KEY-----`),
		witnesses: []string{"-----BEGIN " + "PRIVATE KEY-----\nMIIEow\n-----END PRIVATE KEY-----"},
	},
	{kind: kindAPIKey, re: regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{16,}`), witnesses: vendorKeys([]string{"sk"}, "-", 10)},
	// Stripe's underscore keys, a shape the hyphen anchor of sk- never reaches.
	{
		kind: kindAPIKey, re: regexp.MustCompile(`\b[srp]k_(?:live|test)_[0-9A-Za-z]{16,}`),
		witnesses: append(vendorKeys([]string{"sk_", "rk_", "pk_"}, "live_", 9), vendorKeys([]string{"sk_"}, "test_", 9)...),
	},
	{kind: kindAPIKey, re: regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9]{20,}`), witnesses: vendorKeys([]string{"ghp", "gho", "ghu", "ghs", "ghr"}, "_", 11)},
	{kind: kindAPIKey, re: regexp.MustCompile(`\bxox[baprs]-[A-Za-z0-9-]{10,}`), witnesses: vendorKeys([]string{"xoxb", "xoxa", "xoxp", "xoxr", "xoxs"}, "-", 6)},
	{kind: kindAPIKey, re: regexp.MustCompile(`\bAIza[0-9A-Za-z_-]{30,}`), witnesses: vendorKeys([]string{"AI"}, "za", 18)},
	{
		kind: "aws_access_key", re: regexp.MustCompile(`\b(?:AKIA|ASIA)[0-9A-Z]{16}\b`),
		witnesses: []string{"AK" + "IA" + "IOSFODNN7EXAMPLE", "AS" + "IA" + "IOSFODNN7EXAMPLE"},
	},
	// The 40-character AWS secret has no prefix of its own. Its key name is one
	// underscore-joined word, so the assignment rule's \b never fires in it.
	{
		kind: "aws_secret_key", keepPrefix: true,
		re:        regexp.MustCompile(`(?i)\b((?:aws[_-]?)?secret[_-]?access[_-]?key["']?\s*[:=]\s*["']?)([A-Za-z0-9/+]{40})`),
		witnesses: []string{"aws_secret_access_key = " + strings.Repeat("wJalrXUtnF", 4), "secret-access-key: " + strings.Repeat("K7MDENGbPx", 4)},
	},
	// scheme://user:PASSWORD@host. The closing @ is kept, so host:port after it
	// is never read as user:password.
	{
		kind: "url_credential", keepPrefix: true, keepSuffix: true,
		re: regexp.MustCompile(`(://[^/\s:@"'\\]+:)([^/\s@"'\\]{2,})(@)`),
		// Split like the private-key witness above. A secret scanner reads this
		// file for what it looks like, not for what it is.
		//
		// A whole connection URI here reads as a committed password.
		witnesses: []string{"postgres://svc:" + "S3cr3tPass" + "@db.internal:5432/prod"},
	},
	// Before the bearer rule, so the kind names what was caught.
	{
		kind: "jwt", re: regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}`),
		witnesses: []string{"ey" + "JhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N"},
	},
	{kind: "bearer_token", re: regexp.MustCompile(`(?i)\bbearer[ \t]+[A-Za-z0-9._~+/=-]{16,}`), witnesses: vendorKeys([]string{"Bearer"}, " ", 10)},
	// The value stops at whitespace, quotes and separators, so only the secret goes.
	{
		kind: kindCredentialAssignment, keepPrefix: true, lane: jsonOnly,
		re:        regexp.MustCompile(`(?i)\b((?:` + jsonLabels + `)["']?\s*[:=]\s*["']?)([^\s"'\\,;&]{4,})`),
		witnesses: witnessesOf(jsonWords, `"%s": "hunter2x"`),
	},
	// A quoted passphrase is masked through its closing quote. An unquoted one, or
	// one whose quote never closes on its line, is masked to the end of the line.
	{
		kind: kindCredentialAssignment, keepPrefix: true, keepSuffix: true, lane: textOnly,
		re: regexp.MustCompile(labelled(passwordLabels) + `")([^"\r\n]+)(")`), witnesses: witnessesOf(passwordWords, `%s: "correct horse" ok`),
	},
	{
		kind: kindCredentialAssignment, keepPrefix: true, keepSuffix: true, lane: textOnly,
		re: regexp.MustCompile(labelled(passwordLabels) + `')([^'\r\n]+)(')`), witnesses: witnessesOf(passwordWords, `%s: 'correct horse' ok`),
	},
	{
		kind: kindCredentialAssignment, keepPrefix: true, keepSuffix: true, lane: textOnly,
		re: toLineEnd(passwordLabels, `"`, `[^"\r\n]+?`), witnesses: witnessesOf(passwordWords, "%s: \"open quote\nnext"),
	},
	{
		kind: kindCredentialAssignment, keepPrefix: true, keepSuffix: true, lane: textOnly,
		re: toLineEnd(passwordLabels, `'`, `[^'\r\n]+?`), witnesses: witnessesOf(passwordWords, "%s: 'open quote\nnext"),
	},
	{
		kind: kindCredentialAssignment, keepPrefix: true, keepSuffix: true, lane: textOnly,
		re: toLineEnd(passwordLabels, ``, `[^\s"'][^\r\n]*?`), witnesses: witnessesOf(passwordWords, "%s: Quokka7-x9!"),
	},
	{
		kind: kindCredentialAssignment, keepPrefix: true, lane: textOnly,
		re:        regexp.MustCompile(labelled(strongLabels) + `["']?)([^\s"'\\,;&]{4,})`),
		witnesses: witnessesOf(strongWords, "%s: abcd1234efgh"),
	},
	{
		kind: kindCredentialAssignment, keepPrefix: true, lane: textOnly, accept: looksLikeCredential,
		re:        regexp.MustCompile(labelled(ambiguousLabels) + `["']?)([^\s"'\\,;&]+)`),
		witnesses: witnessesOf(ambiguousWords, "%s: abcd1234efgh"),
	},
	{
		kind: "credential_next_line", keepPrefix: true, keepSuffix: true, lane: textOnly,
		re: nextLine(passwordLabels + `|` + strongLabels), witnesses: witnessesOf(append(slices.Clone(passwordWords), strongWords...), "%s:\nQuokka7x9"),
	},
	{
		kind: "credential_next_line", keepPrefix: true, keepSuffix: true, lane: textOnly, accept: looksLikeCredential,
		re: nextLine(ambiguousLabels), witnesses: witnessesOf(ambiguousWords, "%s:\nabcd1234efgh"),
	},
	// The login on the line above a password line: the pair is the credential.
	{
		kind: "login_name", keepPrefix: true, keepSuffix: true, lane: textOnly,
		re:        regexp.MustCompile(`(?im)^([ \t]*(?:` + loginLabels + `)[ \t]*[:=][ \t]*)([^\s]{2,})([ \t]*\r?\n[ \t]*(?:` + passwordLabels + `)\b)`),
		witnesses: loginWitnesses(),
	},
})

func withCues(rules []rule) []rule {
	for i := range rules {
		rules[i].cues = cuesOf(rules[i].re.String())
	}
	return rules
}
