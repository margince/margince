// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package langcopytest

import (
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

const plantedGate = `
const contraction = /\b(?:can)['’]t\b/i;
it("never speaks as we", () => {
  expect(
    offenders(/\b(?:We|we)\b/, {}) });
`

func TestAPatternIsTheLiteralRightAfterItsLead(t *testing.T) {
	t.Parallel()
	contraction := compiledAfter(t, plantedGate, "const contraction =")
	we := compiledAfter(t, plantedGate, `it("never speaks as we", () => { expect( offenders(`)
	if !contraction.MatchString("CAN'T") || we.MatchString("US") {
		t.Error("a pattern lost its flags on the way from the gate")
	}
	for name, gate := range map[string]string{
		"a lead that stands twice":      plantedGate + plantedGate,
		"a lead that is gone":           strings.Replace(plantedGate, "const contraction", "const english", 1),
		"code between lead and literal": strings.Replace(plantedGate, "= /", "= pick(/", 1),
	} {
		if _, err := patternAfter(gate, "const contraction ="); err == nil {
			t.Errorf("%s was read as the contraction pattern", name)
		}
	}
	reflexive := compiledAfter(t, `const REFLEXIVE = word("(?:chính|tự) mình", "giu");`, "const REFLEXIVE =")
	if !reflexive.MatchString("Tự mình") || reflexive.MatchString("tự mìnhx") {
		t.Error("a word() call was not read as the census reads it")
	}
	if _, err := patternAfter("const contraction = /a(?<!b)c/;", "const contraction ="); err == nil {
		t.Error("a pattern Go cannot compile was read into the snapshot")
	}
}

func compiledAfter(t *testing.T, source, lead string) *regexp.Regexp {
	t.Helper()
	pattern, err := patternAfter(source, lead)
	if err != nil {
		t.Fatal(err)
	}
	compiled, err := pattern.compile()
	if err != nil {
		t.Fatal(err)
	}
	return compiled
}

func TestAJavaScriptPatternReadsAsItDoesInJavaScript(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		source, flags, text string
		want                bool
	}{
		{`(?<!q)o[àá](?![\p{L}])`, "iu", "hoà", true},
		{`(?<!q)o[àá](?![\p{L}])`, "iu", "quà", false},
		{`(?<!q)o[àá](?![\p{L}])`, "iu", "oàn", false},
		{`(?<!q)o[àá](?![\p{L}])`, "iu", "OÀ", true},
		{`x|(?<!q)u[ỳý](?![\p{L}])`, "iu", "tuỳ", true},
		{`[^\s“”(),]+\.[a-z]{1,4}(?![\p{L}\d])`, "gu", "report.pdf", true},
		{`[^\s“”(),]+\.[a-z]{1,4}(?![\p{L}\d])`, "gu", "report.pdfxyz", false},
		{`\s-\s`, "", "a\u00a0-\u00a0b", true},
		{`[^\s]+`, "u", "\u00a0", false},
	} {
		pattern, err := jsPattern(c.source, c.flags)
		if err != nil {
			t.Errorf("/%s/%s: %v", c.source, c.flags, err)
			continue
		}
		if got := pattern.MatchString(c.text); got != c.want {
			t.Errorf("/%s/%s on %q = %v, JavaScript answers %v", c.source, c.flags, c.text, got, c.want)
		}
	}
	for source, flags := range map[string]string{
		`x`:                         "y",
		`\p{L}`:                     "i",
		`anh chị(?! em)`:            "iu",
		`(?<!ab)c`:                  "iu",
		`a(?<!b)c`:                  "iu",
		`(?=x)y`:                    "iu",
		`(?:quý vị|anh chị(?! em))`: "iu",
	} {
		if _, err := jsPattern(source, flags); err == nil {
			t.Errorf("/%s/%s has no Go reading and was compiled", source, flags)
		}
	}
}

func TestAHandPortFailsWhenItsCensusCodeChanges(t *testing.T) {
	t.Parallel()
	census := "matching(\n  \"never addresses anyone\",\n  word(\"quý vị|anh chị(?! em)\"),\n)"
	if err := pinned(census, `"never addresses anyone",`, `word("quý vị|anh chị(?! em)"),`); err != nil {
		t.Errorf("the pinned code, spaced as the census spaces it, was refused: %v", err)
	}
	changed := strings.Replace(census, "(?! em)", "(?! em|$)", 1)
	if err := pinned(changed, `"never addresses anyone",`, `word("quý vị|anh chị(?! em)"),`); err == nil {
		t.Error("a changed census pattern still matched its pin, so the hand port would drift unseen")
	}
}

const plantedPage = "# Page\n\n## Vocabulary\n\n" +
	"| Concept | Vietnamese | Never |\n|---|---|---|\n" +
	"| Sales object | deal | `thương vụ`; giao dịch for a deal |\n" +
	"| Seat | tài khoản | `suất` (hiệu suất and xác suất keep theirs) |\n" +
	"| Self | bạn | `mình` (tự mình keeps it) |\n" +
	"| File | tệp | `file` |\n\n## Next\n| a | b | `ignored` |\n"

func TestTheRetiredWordsAreReadFromTheVocabularyTable(t *testing.T) {
	t.Parallel()
	words, kept, err := vietnameseVocabulary(plantedPage)
	if err != nil {
		t.Fatal(err)
	}
	retired, ordinary, err := vocabulary(words, kept)
	if err != nil {
		t.Fatal(err)
	}
	for text, want := range map[string]bool{
		"thương vụ mới": true, "giao dịch": false, "một suất": true, "hai files": true,
		"ignored": false, "hiệu suất": false, "hiệu suất xác suất": false, "tự mình tự mình": false,
		"hiệu suất một suất": true,
	} {
		if got := retired.MatchString(erase(ordinary, text)); got != want {
			t.Errorf("%q retired = %v, want %v", text, got, want)
		}
	}
	for name, page := range map[string]string{
		"a Never column with no word in code": strings.ReplaceAll(plantedPage, "`", ""),
		"a code span that lost a backtick":    strings.Replace(plantedPage, "`file`", "`file", 1),
		"a row that lost a cell":              strings.Replace(plantedPage, "| File | tệp |", "| File tệp |", 1),
		"a page with no Vocabulary section":   strings.Replace(plantedPage, "## Vocabulary", "## Words", 1),
	} {
		if _, _, err := vietnameseVocabulary(page); err == nil {
			t.Errorf("%s was read as a complete table", name)
		}
	}
}

func TestAnAdjacentExemptPhraseIsErasedToo(t *testing.T) {
	t.Parallel()
	assertStyle(t, []styleCase{
		{"Họ tự mình tự mình quyết định", ""},
		{"Anh chị em anh chị em của họ", ""},
	}, func(r reporter, text string) { VietnameseText(r, "Field", text) })
}

func TestTheStyleSourcesAreTheRepositorysOwn(t *testing.T) {
	t.Parallel()
	src, err := loadSources()
	if err != nil {
		t.Fatal(err)
	}
	if want := len(englishPatternLeads) + len(vietnamesePatternLeads); len(src.patterns) != want {
		t.Errorf("compiled %d patterns, want %d", len(src.patterns), want)
	}
	if !src.retired.MatchString("thương vụ") {
		t.Errorf("%s no longer retires thương vụ, or the snapshot lost the table", vietnamesePagePath)
	}
	for _, rule := range styleRules {
		for _, text := range []string{"", "Thương vụ “x” report.pdf https://x.y {n} %d tôi", "We can't"} {
			rule.breaks(src, text, "my")
		}
	}
}

func TestABaselineLineIsARuleAndAQuotedSentence(t *testing.T) {
	t.Parallel()
	waivers, err := parseBaseline("# header\n\nen.we\t\"We saved\\tit\"\n")
	if err != nil || len(waivers) != 1 || waivers[0] != (Waiver{Rule: ruleEnWe, Text: "We saved\tit"}) {
		t.Fatalf("parsed %v, %v", waivers, err)
	}
	for _, line := range []string{"en.we We saved it", "vi.toi\t\"Tôi đã lưu\"", "en.we\t\"We\"\t\"We\""} {
		if _, err := parseBaseline(line + "\n"); err == nil {
			t.Errorf("%q was read as a waiver", line)
		}
	}
	waivers, err = parseBaseline("vi.toi\t\"Giao cho tôi\"\t\"Assign\"\n")
	if err != nil || waivers[0].English != "Assign" {
		t.Errorf("a vi.toi waiver lost the English it was checked beside: %v, %v", waivers, err)
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
	if rule.readsEnglish {
		return
	}
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
