// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package credentialmask

import (
	"encoding/json"
	"regexp"
	"regexp/syntax"
	"slices"
	"strings"
	"testing"
	"unicode"
)

// labelWords spells every word a label pattern matches. It walks the pattern
// without the cue derivation. A cue the derivation drops then shows up as a
// label line the masker no longer masks.
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
// without it. That holds in every case a reader may type, the Kelvin sign too.
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
func TestEveryRuleHasCuesAndWitnesses(t *testing.T) {
	for i, r := range rules {
		if len(r.cues) == 0 {
			t.Errorf("rule %d (%s) has no cues: %s", i, r.kind, r.re)
		}
		if len(r.witnesses) == 0 {
			t.Errorf("rule %d (%s) has no witnesses: %s", i, r.kind, r.re)
		}
	}
}

// witnessForms are the ways a witness reaches the masker: alone, inside prose
// and in another case. For the JSON lane each is also a string in a document.
func witnessForms(t *testing.T, witness string, lane Lane) []string {
	t.Helper()
	cased := []string{witness, strings.ToUpper(witness), strings.ToLower(witness), mixedCase(witness)}
	var forms []string
	for _, text := range cased {
		forms = append(forms, text, "Hello team,\n"+text+"\nthanks, Anna")
	}
	if lane == Text {
		return forms
	}
	jsonForms := slices.Clone(cased)
	for _, text := range forms {
		encoded, err := json.Marshal(text)
		if err != nil {
			t.Fatal(err)
		}
		jsonForms = append(jsonForms, `{"content": `+string(encoded)+`}`)
	}
	return jsonForms
}

func mixedCase(s string) string {
	runes := []rune(s)
	for i := range runes {
		if i%2 == 1 {
			runes[i] = unicode.ToUpper(runes[i])
		} else {
			runes[i] = unicode.ToLower(runes[i])
		}
	}
	return string(runes)
}

// Each witness of each rule is a match of the rule. Every form of it the rule
// matches holds one of the rule's cues. The masker answers the same with the
// cue check as without it.
func TestEveryWitnessHoldsItsRulesCues(t *testing.T) {
	mark := func(kind string) string { return "<" + kind + ">" }
	for i, r := range rules {
		for _, witness := range r.witnesses {
			if !r.re.MatchString(witness) {
				t.Errorf("rule %d (%s): witness %q is no match of %s", i, r.kind, witness, r.re)
			}
			for _, lane := range []Lane{Text, JSON} {
				if !r.runsIn(lane) {
					continue
				}
				for _, form := range witnessForms(t, witness, lane) {
					if r.re.MatchString(form) && !newCueScan([]byte(form)).mayMatch(r.cues) {
						t.Errorf("rule %d (%s): %q matches and holds none of its cues", i, r.kind, form)
					}
					cued, _ := replaceCued([]byte(form), lane, mark, true)
					full, _ := replaceCued([]byte(form), lane, mark, false)
					if string(cued) != string(full) {
						t.Errorf("rule %d (%s), lane %d, %q: with cues %q, without %q", i, r.kind, lane, form, cued, full)
					}
				}
			}
		}
	}
}

// A witness set that leaves an alternation branch untaken leaves that branch's
// cue unchecked. Each branch is pinned in turn and some witness must match the
// pinned pattern.
func TestTheWitnessesTakeEveryAlternationBranch(t *testing.T) {
	for i, r := range rules {
		root, err := syntax.Parse(r.re.String(), syntax.Perl)
		if err != nil {
			t.Fatalf("rule %d: %v", i, err)
		}
		for _, alternation := range alternations(root) {
			whole := *alternation
			for b, branch := range whole.Sub {
				*alternation = *branch
				pinned := regexp.MustCompile(root.String())
				*alternation = whole
				if !slices.ContainsFunc(r.witnesses, pinned.MatchString) {
					t.Errorf("rule %d (%s): no witness takes branch %d %q of %s", i, r.kind, b, branch, &whole)
				}
			}
		}
	}
}

func alternations(re *syntax.Regexp) []*syntax.Regexp {
	var found []*syntax.Regexp
	if re.Op == syntax.OpAlternate {
		found = append(found, re)
	}
	for _, sub := range re.Sub {
		found = append(found, alternations(sub)...)
	}
	return found
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
