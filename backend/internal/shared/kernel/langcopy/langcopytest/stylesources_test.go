// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package langcopytest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

const plantedGate = `
const contraction = /\b(?:can)['’]t\b/i;
it("never speaks as we", () => { offenders(/\b(?:We|we)\b/, {}) });
it("says I only inside the onboarding conversation", () => { offenders(/\bI\b/, {}) });
const british = /\b(?:colour\w*)\b/i;
`

func TestTheEnglishPatternsAreReadFromTheCatalogGate(t *testing.T) {
	t.Parallel()
	patterns, err := englishPatterns(plantedGate)
	if err != nil {
		t.Fatal(err)
	}
	if !patterns[ruleEnContraction].MatchString("CAN'T") || patterns[ruleEnWe].MatchString("US") {
		t.Error("a pattern lost its flags on the way from the gate")
	}
	if _, err := englishPatterns(strings.Replace(plantedGate, "const british", "const english", 1)); err == nil {
		t.Error("a gate without the British list was read as complete")
	}
	if _, err := jsRegex("x", "g"); err == nil {
		t.Error("a flag with no Go reading was dropped instead of refused")
	}
}

const plantedPage = "# Page\n\n## Vocabulary\n\n" +
	"| Concept | Vietnamese | Never |\n|---|---|---|\n" +
	"| Sales object | deal | `thương vụ`; giao dịch for a deal |\n" +
	"| Seat | tài khoản | `suất` (hiệu suất and xác suất keep theirs) |\n" +
	"| File | tệp | `file` |\n\n## Next\n| a | b | `ignored` |\n"

func TestTheRetiredWordsAreReadFromTheVocabularyTable(t *testing.T) {
	t.Parallel()
	retired, ordinary, err := vietnameseVocabulary(plantedPage)
	if err != nil {
		t.Fatal(err)
	}
	for text, want := range map[string]bool{
		"thương vụ mới": true, "giao dịch": false, "một suất": true, "hai files": true,
		"ignored": false, "hiệu suất": false,
	} {
		if got := retired.MatchString(ordinary.ReplaceAllString(text, " ")); got != want {
			t.Errorf("%q retired = %v, want %v", text, got, want)
		}
	}
	if _, _, err := vietnameseVocabulary(strings.ReplaceAll(plantedPage, "`", "")); err == nil {
		t.Error("a Never column with no word in code was read as retiring nothing")
	}
}

func TestTheStyleSourcesAreTheRepositorysOwn(t *testing.T) {
	t.Parallel()
	src, err := loadSources()
	if err != nil {
		t.Fatal(err)
	}
	if len(src.english) != len(englishPatternAnchors) {
		t.Errorf("read %d English patterns from %s, want %d", len(src.english), englishGatePath, len(englishPatternAnchors))
	}
	if !src.retired.MatchString("thương vụ") {
		t.Errorf("%s no longer retires thương vụ, or the table is not being read", vietnamesePagePath)
	}
}

func TestABaselineLineIsARuleAndAQuotedSentence(t *testing.T) {
	t.Parallel()
	waivers, err := parseBaseline("# header\n\nen.we\t\"We saved\\tit\"\n")
	if err != nil || len(waivers) != 1 || waivers[0] != (Waiver{Rule: ruleEnWe, Text: "We saved\tit"}) {
		t.Fatalf("parsed %v, %v", waivers, err)
	}
	if _, err := parseBaseline("en.we We saved it\n"); err == nil {
		t.Error("a line with no tab and no quotes was read as a waiver")
	}
}

func TestABaselineEntryWaivesItsFindingAlone(t *testing.T) {
	t.Parallel()
	waivers, err := StyleBaseline()
	if err != nil {
		t.Fatal(err)
	}
	if len(waivers) == 0 {
		return
	}
	w := waivers[0]
	rule, _ := ruleNamed(w.Rule)
	check := VietnameseText
	if rule.lang == textlang.English {
		check = EnglishText
	}
	for _, got := range rulesReported(func(r reporter) { check(r, "Field", w.Text) }) {
		if got == w.Rule {
			t.Errorf("%s still reports %q, which the baseline waives", w.Rule, w.Text)
		}
	}
	if got := rulesReported(func(r reporter) { check(r, "Field", w.Text+" ") }); !strings.Contains(strings.Join(got, " "), w.Rule) {
		t.Errorf("a waiver for %q also waived the same words with a trailing space: %v", w.Text, got)
	}
}

func TestARecutRecordsEveryFindingInsteadOfFailing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "findings")
	t.Setenv(recordVariable, path)
	if got := rulesReported(func(r reporter) { EnglishText(r, "Field", "We saved it") }); len(got) != 0 {
		t.Errorf("a recording run failed on %v", got)
	}
	recorded, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(recorded), ruleEnWe+"\t\"We saved it\"\t") {
		t.Errorf("recorded %q", recorded)
	}
}
