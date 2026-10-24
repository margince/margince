// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// The browser reads a phone number with ParsePhone's separators, dial prefix and E.164 pattern.
//
// contacturi.ts offers a messenger chat only for a number it reads as E.164. If its rules
// drift from the server's, it links numbers the server refuses, or hides the link for stored ones.

import (
	"os"
	"regexp"
	"regexp/syntax"
	"slices"
	"strconv"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

const frontendContactURI = "../frontend/src/format/contacturi.ts"

// tsDialPrefixRule is the 00 rule as contacturi.ts spells it: startsWith("00"), then "+" and slice(2).
var tsDialPrefixRule = regexp.MustCompile("\\.startsWith\\(\"(\\d+)\"\\)\\s*\\?\\s*`\\+\\$\\{\\w+\\.slice\\((\\d+)\\)\\}`")

func TestTheFrontendPhoneParseMatchesParsePhone(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile(frontendContactURI)
	if err != nil {
		t.Fatalf("reading the frontend phone mirror: %v", err)
	}
	source := string(raw)
	body := tsFunctionBody(t, source, "e164Digits")
	for _, use := range []string{`\.replace\(\s*PHONE_SEPARATORS\s*,`, `\bE164\.(?:exec|test)\(`} {
		if !regexp.MustCompile(use).MatchString(body) {
			t.Errorf("%s: e164Digits no longer calls /%s/, so the declarations this gate compares are not the ones it runs", frontendContactURI, use)
		}
	}

	goPattern := withoutCaptures(t, "values.E164Pattern", values.E164Pattern)
	tsPattern := withoutCaptures(t, frontendContactURI+" E164", tsRegexLiteral(t, source, "E164"))
	if goPattern != tsPattern {
		t.Errorf("E.164 drifted: values.E164Pattern reads %s, %s E164 reads %s. Change the side that moved", goPattern, frontendContactURI, tsPattern)
	}

	compareSeparators(t, tsRegexLiteral(t, source, "PHONE_SEPARATORS"))
	compareDialPrefix(t, body)
}

func compareSeparators(t *testing.T, tsClass string) {
	t.Helper()
	parsed, err := syntax.Parse(tsClass, syntax.Perl)
	if err != nil || parsed.Op != syntax.OpCharClass {
		t.Fatalf("%s: PHONE_SEPARATORS %q is no longer one character class (%v); the gate cannot read it", frontendContactURI, tsClass, err)
	}
	var inTS []rune
	for i := 0; i < len(parsed.Rune); i += 2 {
		for r := parsed.Rune[i]; r <= parsed.Rune[i+1]; r++ {
			inTS = append(inTS, r)
		}
	}
	var inGo []rune
	for _, separator := range values.PhoneSeparators() {
		runes := []rune(separator)
		if len(runes) != 1 {
			t.Fatalf("values.PhoneSeparators holds %q; a character class can only mirror single characters", separator)
		}
		inGo = append(inGo, runes[0])
		if got, parseErr := values.ParsePhone("+49" + separator + "30123456"); parseErr != nil || got.String() != "+4930123456" {
			t.Errorf("ParsePhone does not strip %q, which values.PhoneSeparators lists; build its replacer from that list", separator)
		}
	}
	for _, r := range inGo {
		if !slices.Contains(inTS, r) {
			t.Errorf("separator %q: ParsePhone strips it and %s PHONE_SEPARATORS does not. Add it there or drop it from values.PhoneSeparators", r, frontendContactURI)
		}
	}
	for _, r := range inTS {
		if !slices.Contains(inGo, r) {
			t.Errorf("separator %q: %s PHONE_SEPARATORS strips it and ParsePhone does not. Drop it there or add it to values.PhoneSeparators", r, frontendContactURI)
		}
	}
}

func compareDialPrefix(t *testing.T, e164DigitsBody string) {
	t.Helper()
	prefix := values.InternationalDialPrefix
	if got, err := values.ParsePhone(prefix + "4930123456"); err != nil || got.String() != "+4930123456" {
		t.Errorf("ParsePhone no longer reads a leading %q as \"+\", and %s still does. Restore it in phone.go or drop it in e164Digits", prefix, frontendContactURI)
	}
	rule := tsDialPrefixRule.FindStringSubmatch(e164DigitsBody)
	if rule == nil {
		t.Errorf("%s: e164Digits no longer reads a leading %q as \"+\", and ParsePhone does. Restore `startsWith(%q) ? `+${x.slice(%d)}``", frontendContactURI, prefix, prefix, len(prefix))
		return
	}
	if sliced, err := strconv.Atoi(rule[2]); rule[1] != prefix || err != nil || sliced != len(prefix) {
		t.Errorf("%s: e164Digits swaps %q (slicing %s) for \"+\"; values.InternationalDialPrefix is %q", frontendContactURI, rule[1], rule[2], prefix)
	}
}

// tsRegexLiteral is the body of the one `const name = /body/flags;` statement in source.
func tsRegexLiteral(t *testing.T, source, name string) string {
	t.Helper()
	statement := regexp.MustCompile(`(?m)^(?:export\s+)?const\s+` + regexp.QuoteMeta(name) + `\s*=\s*/(.+)/[a-z]*;\s*$`)
	found := statement.FindAllStringSubmatch(source, -1)
	if len(found) != 1 {
		t.Fatalf("%s: found %d `const %s = /…/;` statements, want 1. A rename makes this gate read nothing; rename it here too", frontendContactURI, len(found), name)
	}
	return found[0][1]
}

func tsFunctionBody(t *testing.T, source, name string) string {
	t.Helper()
	start := indexAfter(source, "\nfunction "+name+"(")
	if start < 0 {
		t.Fatalf("%s no longer declares function %s; the gate cannot see how the mirror is applied", frontendContactURI, name)
	}
	end := indexAfter(source[start:], "\n}\n")
	if end < 0 {
		t.Fatalf("%s: function %s is unterminated", frontendContactURI, name)
	}
	return source[start : start+end]
}

// withoutCaptures parses pattern so `\d` and `[0-9]` compare equal, and drops the capture TS needs.
func withoutCaptures(t *testing.T, origin, pattern string) string {
	t.Helper()
	parsed, err := syntax.Parse(pattern, syntax.Perl)
	if err != nil {
		t.Fatalf("%s: %q does not parse as a regular expression: %v", origin, pattern, err)
	}
	return flattenCaptures(parsed).String()
}

func flattenCaptures(node *syntax.Regexp) *syntax.Regexp {
	var subs []*syntax.Regexp
	for _, sub := range node.Sub {
		sub = flattenCaptures(sub)
		if node.Op == syntax.OpConcat && sub.Op == syntax.OpConcat {
			subs = append(subs, sub.Sub...)
			continue
		}
		subs = append(subs, sub)
	}
	node.Sub = subs
	if node.Op == syntax.OpCapture {
		return node.Sub[0]
	}
	return node
}
