// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package credentialmask

import (
	"regexp/syntax"
	"strings"
	"testing"
)

// labelWords spells every word a label pattern matches. It walks the pattern
// on its own, without the cue derivation, so a cue the derivation drops shows
// up as a label line the masker no longer masks.
func labelWords(t *testing.T, pattern string) []string {
	t.Helper()
	re, err := syntax.Parse(pattern, syntax.Perl)
	if err != nil {
		t.Fatalf("parsing %q: %v", pattern, err)
	}
	return spell(t, re.Simplify())
}

func spell(t *testing.T, re *syntax.Regexp) []string {
	t.Helper()
	switch re.Op {
	case syntax.OpLiteral:
		return []string{string(re.Rune)}
	case syntax.OpEmptyMatch:
		return []string{""}
	case syntax.OpCharClass:
		var out []string
		for i := 0; i < len(re.Rune); i += 2 {
			for r := re.Rune[i]; r <= re.Rune[i+1]; r++ {
				out = append(out, string(r))
			}
		}
		return out
	case syntax.OpCapture:
		return spell(t, re.Sub[0])
	case syntax.OpQuest:
		return append(spell(t, re.Sub[0]), "")
	case syntax.OpAlternate:
		var out []string
		for _, sub := range re.Sub {
			out = append(out, spell(t, sub)...)
		}
		return out
	case syntax.OpConcat:
		out := []string{""}
		for _, sub := range re.Sub {
			var next []string
			for _, head := range out {
				for _, tail := range spell(t, sub) {
					next = append(next, head+tail)
				}
			}
			out = next
		}
		return out
	default:
		t.Fatalf("a label pattern uses %v, which this test cannot spell", re.Op)
		return nil
	}
}

// Every label line the rules mask is masked the same with the cue check as
// without it, in every case a reader may type it, the Kelvin sign included.
func TestTheCueCheckSkipsNoLabelARuleMatches(t *testing.T) {
	var lines []string
	for _, labels := range []string{passwordLabels, strongLabels, ambiguousLabels, jsonLabels} {
		for _, word := range labelWords(t, labels) {
			for _, spelled := range []string{word, strings.ToUpper(word), strings.ReplaceAll(word, "k", "K")} {
				lines = append(lines,
					spelled+": Quokka7-x9!", spelled+"=Quokka7-x9!", `"`+spelled+`": "Quokka7-x9!"`,
					spelled+":\nQuokka7-x9!", "Login: anna\n"+spelled+": Quokka7-x9!")
			}
		}
	}
	for _, lane := range []Lane{Text, JSON} {
		for _, line := range lines {
			cued, cuedFound := replaceCued([]byte(line), lane, func(string) string { return Marker }, true)
			full, fullFound := replaceCued([]byte(line), lane, func(string) string { return Marker }, false)
			if string(cued) != string(full) || len(cuedFound) != len(fullFound) {
				t.Errorf("lane %d, %q: with cues %q, without %q", lane, line, cued, full)
			}
		}
	}
}

// A rule with no cues runs on every text, which is safe and gives up the
// speed the check is for.
func TestEveryRuleHasCues(t *testing.T) {
	for i, r := range rules {
		if len(r.cues) == 0 {
			t.Errorf("rule %d (%s) has no cues: %s", i, r.kind, r.re)
		}
	}
}

func TestTheCueCheckAgreesOnEveryMaskedShape(t *testing.T) {
	texts := []string{
		"use " + stripeSecret + " now", "ghp_16C7e42F292c6912E7710c838347Ae178B4a",
		"id AKIAIOSFODNN7EXAMPLE here", "postgres://svc:S3cr3tPass@db.internal:5432/prod",
		"Authorization: Bearer abcdefghijklmnop1234", "key:\n-----BEGIN PRIVATE KEY-----\nMIIEow\n-----END PRIVATE KEY-----",
		"aws_secret_access_key = wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY", "Spin: 4711",
		"eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U",
		"xoxb-1234567890-abcdefghij", "AIza" + strings.Repeat("b", 35),
	}
	for _, text := range texts {
		cued, _ := replaceCued([]byte(text), Text, func(kind string) string { return "[SECRET-REMOVED:" + kind + "]" }, true)
		full, _ := replaceCued([]byte(text), Text, func(kind string) string { return "[SECRET-REMOVED:" + kind + "]" }, false)
		if string(cued) != string(full) {
			t.Errorf("%q: with cues %q, without %q", text, cued, full)
		}
	}
}

func BenchmarkMaskTextOnMailWithNoCredential(b *testing.B) {
	body := strings.Repeat("Hello, thanks for the call today about pricing.\n", 400)
	for b.Loop() {
		MaskText(body)
	}
}

func BenchmarkMaskTextOnMailWithNoCredentialUncued(b *testing.B) {
	body := []byte(strings.Repeat("Hello, thanks for the call today about pricing.\n", 400))
	for b.Loop() {
		replaceCued(body, Text, func(string) string { return Marker }, false)
	}
}
