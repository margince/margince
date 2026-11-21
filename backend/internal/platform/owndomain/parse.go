// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package owndomain

import (
	"strings"
	"unicode"

	"golang.org/x/net/idna"

	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// ParseDomain is values.ParseDomain plus the IDNA fold. A Unicode domain is
// stored as punycode, so münchen.de and xn--mnchen-3ya.de are one company
// domain, in the form mail senders are matched in.
//
// A label that mixes scripts, or that IDNA refuses, is not a domain.
func ParseDomain(raw string) (values.Domain, error) {
	parsed, err := values.ParseDomain(raw)
	if err != nil {
		return values.Domain{}, err
	}
	ascii, err := idna.Lookup.ToASCII(parsed.String())
	if err != nil {
		return values.Domain{}, malformedDomain()
	}
	// Checked on the decoded form, so a punycode spelling cannot hide a
	// look-alike label or an invisible character.
	decoded, err := idna.Lookup.ToUnicode(ascii)
	if err != nil || !plainLetters(decoded) || mixedScript(decoded) {
		return values.Domain{}, malformedDomain()
	}
	return values.ParseDomain(ascii)
}

// plainLetters admits only letters, marks, digits, hyphens and dots, which
// leaves out bidi controls, zero-width characters and other format characters.
func plainLetters(host string) bool {
	for _, r := range host {
		if r != '-' && r != '.' && !unicode.IsLetter(r) && !unicode.IsMark(r) && !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

func malformedDomain() error {
	return &values.ParseError{
		Field: "domain", Code: "domain_malformed",
		Message: "not a host name (labels of letters, digits and inner hyphens, with a dot)",
	}
}

// mixedScript reports a label whose letters come from more than one script,
// the look-alike trick of a Cyrillic "а" inside a Latin name. Han, Kana and
// Hangul count as one script because those languages write them together.
func mixedScript(host string) bool {
	for part := range strings.SplitSeq(host, ".") {
		if scriptOf(part) == mixed {
			return true
		}
	}
	return false
}

const mixed = "mixed"

func scriptOf(label string) string {
	found := ""
	for _, r := range label {
		if !unicode.IsLetter(r) {
			continue
		}
		s := scriptName(r)
		if found != "" && s != found {
			return mixed
		}
		found = s
	}
	return found
}

func scriptName(r rune) string {
	for name, table := range unicode.Scripts {
		if !unicode.Is(table, r) {
			continue
		}
		switch name {
		case "Han", "Hiragana", "Katakana", "Hangul", "Bopomofo":
			return "CJK"
		}
		return name
	}
	return ""
}

// DomainOrAsWritten is the stored form of a domain, or the trimmed input when
// it is no domain. A comparison uses it so a bad cell is not refused.
func DomainOrAsWritten(raw string) string {
	if parsed, err := ParseDomain(raw); err == nil {
		return parsed.String()
	}
	return strings.TrimSpace(raw)
}

// KeyForm is a company routing key in its stored form. A key shaped like a
// domain is reduced to the punycode host ParseDomain stores; any other key (a
// slug) is left as typed.
func KeyForm(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if parsed, err := ParseDomain(trimmed); err == nil {
		return parsed.String()
	}
	return trimmed
}

// KeySpellings returns the lowercase spellings a stored routing key may carry
// for raw: the punycode form and its Unicode twin for a domain-shaped key.
// Rows written before keys were folded may hold either.
func KeySpellings(raw string) []string {
	form := KeyForm(raw)
	if _, err := ParseDomain(form); err != nil {
		return []string{strings.ToLower(form)}
	}
	return Spellings(form)
}
