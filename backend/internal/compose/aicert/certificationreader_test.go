// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert_test

// The top of the certification page: the half written for somebody choosing a
// preset, who asks "can I use it?" and has never heard of a task id, a stamp, a
// band or a judge. Everything here is layout over the same document the
// engineers' half renders — the grades are the records' own verdicts, and the
// grading summary quotes the verdict rule's own constants and doc comment, so
// this half cannot hold a second opinion about any of them.

import (
	"fmt"
	"go/parser"
	"go/token"
	"sort"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/aicert"
	"github.com/margince/margince/backend/internal/modules/ai"
)

// aiCertWhereDataGoes names a preset's profile as the place a reader's data is
// sent. A profile it does not name answers empty, which
// assertAICertProfilesAreNamed fails rather than letting a blank cell ship.
func aiCertWhereDataGoes(p aiCertPreset) string {
	switch ai.Profile(p.Profile) {
	case ai.ProfileEUHosted:
		return "EU-hosted cloud"
	case ai.ProfileSovereign:
		return "your own servers"
	case ai.ProfileCloudFrontier:
		return "global cloud"
	default:
		return ""
	}
}

// The four grades a reader sees, and the fifth answer a preset can give.
const (
	aiCertReady    = "✅ Ready"
	aiCertCare     = "⚠️ Usable with care"
	aiCertNotYet   = "❌ Not reliable yet"
	aiCertUnproven = "❔ Not measured"
	aiCertOff      = "➖ Off"
)

// aiCertVerdictSource is the file whose Verdict doc comment states the rule.
const aiCertVerdictSource = "score.go"

func assertAICertProfilesAreNamed(t *testing.T, presets []aiCertPreset) {
	t.Helper()
	for _, p := range presets {
		if aiCertWhereDataGoes(p) == "" {
			t.Errorf("preset %s declares profile %q, which aiCertWhereDataGoes does not name — "+
				"the page would not say where that preset sends data", p.File, p.Profile)
		}
	}
}

// loadAICertVerdictRule is the indented rule block of Verdict's doc comment,
// so the page's exact rule is the one the code documents beside itself. It
// fails when the block names a pass rate or majority other than the constants
// the rule reads, which is the one way the two could disagree.
func loadAICertVerdictRule(t *testing.T) string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), aiCertVerdictSource, nil, parser.ParseComments)
	if err != nil {
		t.Fatalf("parsing %s for the verdict rule: %v", aiCertVerdictSource, err)
	}
	var rule []string
	for _, group := range file.Comments {
		text := group.Text()
		if !strings.HasPrefix(text, "Verdict folds") {
			continue
		}
		for _, line := range strings.Split(text, "\n") {
			if strings.HasPrefix(line, "\t") {
				rule = append(rule, strings.TrimPrefix(line, "\t"))
			}
		}
	}
	if len(rule) == 0 {
		t.Fatalf("%s has no indented rule block in Verdict's doc comment; the page quotes it", aiCertVerdictSource)
	}
	joined := strings.Join(rule, "\n")
	if percent := fmt.Sprintf("%d%%", aicert.CertifiedPassPercent); !strings.Contains(joined, percent) {
		t.Fatalf("Verdict's documented rule does not state the %s pass rate the code applies:\n%s", percent, joined)
	}
	if majority := fmt.Sprintf("⌈%dn/%d⌉", aicert.MajorityNumerator, aicert.MajorityDenominator); !strings.Contains(joined, majority) {
		t.Fatalf("Verdict's documented rule does not state the %s case majority the code applies:\n%s", majority, joined)
	}
	return joined
}

// assertAICertPresetSectionsCount reads each preset's rendered section back
// and counts its grades against the document's counts, which the summary row
// prints.
func assertAICertPresetSectionsCount(t *testing.T, page string, presets []aiCertPreset) {
	t.Helper()
	for _, p := range presets {
		heading := "### `" + aiCertPresetName(p) + "`\n"
		start := strings.Index(page, heading)
		if start < 0 {
			t.Errorf("the page has no section for preset %s", p.File)
			continue
		}
		section, _, _ := strings.Cut(page[start:], "<details>")
		want := map[string]int{
			aiCertReady: p.Bands.Certified, aiCertCare: p.Bands.SupportedDegraded,
			aiCertNotYet: p.Bands.NotSupported, aiCertUnproven: p.Untested, aiCertOff: p.Unbound,
		}
		for grade, count := range want {
			if got := strings.Count(section, "| "+grade+" |"); got != count {
				t.Errorf("preset %s's section shows %d feature(s) %s; the document counts %d", p.File, got, grade, count)
			}
		}
		if row := "| [`" + aiCertPresetName(p) + "`]"; !strings.Contains(page, row) {
			t.Errorf("preset %s has no row in the summary table", p.File)
		}
	}
}

func writeAICertHead(page *strings.Builder) {
	page.WriteString("# AI certification\n\n")
	page.WriteString("<!-- Generated from the invocation-site census, the scenario corpus and the committed records; do not edit by hand. -->\n\n")
	page.WriteString("This page tells you which AI features you can rely on under each preset — the\n")
	page.WriteString("ready-made choice of AI models you pick when you set Margince up. Every grade\n")
	page.WriteString("here was measured by running the product's real prompts against the model, not\n")
	page.WriteString("promised by anyone.\n\n")
	page.WriteString("<details>\n<summary>How this page is made</summary>\n\n")
	page.WriteString("Generated by `" + aiCertRegenerate + "`; do not edit by hand. It reads the same\n")
	page.WriteString("three trees `make e2e-ai-report` reads: the sites this build registers, the\n")
	page.WriteString("scenarios under [`backend/internal/compose/aicert/corpus/`](" + corpusLinkPrefix + aiCertCorpusDocs + "corpus/README.md),\n")
	page.WriteString("and the records under [`backend/internal/compose/aicert/records/`](" + corpusLinkPrefix + aiCertCorpusDocs + "records/README.md).\n\n")
	page.WriteString("[`ai-certification.json`](ai-certification.json) beside this page holds the same\n")
	page.WriteString("numbers, whole, for a reader who wants to ask a question this page does not\n")
	page.WriteString("answer. This page is rendered from that file.\n\n")
	page.WriteString("Nothing here is a merge gate. The certification lane is paid, manual and\n")
	page.WriteString("BYOK-gated, so a prompt edit turns a record `stale` rather than failing a build.\n\n")
	page.WriteString("How to add a case: [write-a-certification-case.md](../how-to/write-a-certification-case.md).\n")
	page.WriteString("How to certify a model: [certify-an-ai-model.md](../how-to/certify-an-ai-model.md).\n\n")
	page.WriteString("</details>\n\n")
}

// writeAICertPresetSummary is the page's answer, one row per preset: a reader
// choosing between them reads across a row and down a column, and needs
// nothing else on the page to do it.
func writeAICertPresetSummary(page *strings.Builder, presets []aiCertPreset) {
	page.WriteString("## Can I use this preset?\n\n")
	page.WriteString("A [preset](" + aiCertPresetLink + "README.md) picks which AI model runs each feature, so the same\n")
	page.WriteString("feature can be ready under one preset and not under another.\n\n")
	writeAICertOlderVersionNote(page, presets)
	page.WriteString("| Preset | Where your data goes | " + aiCertReady + " | " + aiCertCare + " | " +
		aiCertNotYet + " | " + aiCertUnproven + " | Bottom line |\n")
	page.WriteString("|---|---|---:|---:|---:|---:|---|\n")
	anyOff := false
	for _, p := range presets {
		name := aiCertPresetName(p)
		fmt.Fprintf(page, "| [`%s`](#%s) | %s | %d | %d | %d | %d | %s |\n",
			name, aiCertSiteAnchor(name), aiCertWhereDataGoes(p),
			p.Bands.Certified, p.Bands.SupportedDegraded, p.Bands.NotSupported, p.Untested, aiCertBottomLine(p))
		anyOff = anyOff || p.Unbound > 0
	}
	page.WriteString("\n")
	writeAICertLegend(page, anyOff)
	for _, p := range presets {
		writeAICertPresetDetail(page, p)
	}
}

// writeAICertLegend says what each grade means for the reader's own decision:
// what was measured, and what to do about it. Off is listed only when a preset
// switches something off, so the legend never explains a mark the page lacks.
func writeAICertLegend(page *strings.Builder, anyOff bool) {
	page.WriteString("**What the grades mean**\n\n")
	page.WriteString("| Grade | What we measured | What to do |\n|---|---|---|\n")
	fmt.Fprintf(page, "| %s | Right in at least %d of every 100 tries, no test case failing again and again, and good answers. | Turn it on and rely on it. |\n",
		aiCertReady, aicert.CertifiedPassPercent)
	fmt.Fprintf(page, "| %s | Right in at least %s of tries, and acceptable answers. | Turn it on, and have someone look over what it produces. |\n",
		aiCertCare, aiCertMajorityWords())
	fmt.Fprintf(page, "| %s | Wrong too often, or answers below the quality bar. | Leave it off, or check every answer by hand. |\n", aiCertNotYet)
	fmt.Fprintf(page, "| %s | This preset has a model for the feature, but nobody has tested it yet. | Ask for a test before relying on it. |\n", aiCertUnproven)
	if anyOff {
		fmt.Fprintf(page, "| %s | This preset has no model for the feature. | Nothing — the feature is switched off. |\n", aiCertOff)
	}
	page.WriteString("\n*re-check pending* after a grade means the product has changed since it was\n")
	page.WriteString("measured. The grade is the last one we have, and it is shown until the next test replaces it.\n")
	page.WriteString("[How the scoring works](#how-the-scoring-works) explains how a grade is reached.\n\n")
}

func aiCertPresetName(p aiCertPreset) string { return strings.TrimSuffix(p.File, ".yaml") }

// aiCertBottomLine is the preset's one-line answer. A count read from stale
// grades says so, or the summary row would state old measurements as current.
func aiCertBottomLine(p aiCertPreset) string {
	line := fmt.Sprintf("%d of %d features ready", p.Bands.Certified, len(p.Tasks))
	measured, stale := aiCertStaleGrades(p)
	switch stale {
	case 0:
	case measured:
		line += " (re-check pending)"
	case 1:
		line += " (1 re-check pending)"
	default:
		line += fmt.Sprintf(" (%d re-checks pending)", stale)
	}
	if p.Unbound > 0 {
		line += fmt.Sprintf(", %d switched off", p.Unbound)
	}
	return line
}

// aiCertStaleGrades counts p's measured grades and how many of them are stale.
func aiCertStaleGrades(p aiCertPreset) (measured, stale int) {
	for _, row := range p.Tasks {
		if row.Band == "" {
			continue
		}
		measured++
		if row.State == aicert.StatusStale {
			stale++
		}
	}
	return measured, stale
}

// writeAICertOlderVersionNote says, once, how many of the grades below were
// measured on a version of the product that has since changed. The grades
// stay shown — they are the last measurement there is — and this is what
// keeps them from reading as current.
func writeAICertOlderVersionNote(page *strings.Builder, presets []aiCertPreset) {
	measured, older := 0, 0
	for _, p := range presets {
		presetMeasured, presetStale := aiCertStaleGrades(p)
		measured += presetMeasured
		older += presetStale
	}
	switch older {
	case 0:
		return
	case measured:
		page.WriteString("Every grade below was measured on an older version of the product, so each one\n")
		page.WriteString("is waiting to be re-checked.\n\n")
	default:
		fmt.Fprintf(page, "%d of the %d grades below were measured on an older version of the product and\n"+
			"are waiting to be re-checked; each is marked below.\n\n", older, measured)
	}
}

// writeAICertPresetDetail is one preset's features, in words, with the models
// behind them folded away for the reader who wants them.
func writeAICertPresetDetail(page *strings.Builder, p aiCertPreset) {
	fmt.Fprintf(page, "### `%s`\n\n", aiCertPresetName(p))
	fmt.Fprintf(page, "Your data goes to: %s. %s. Preset file: [`%s`](%s%s).\n\n",
		aiCertWhereDataGoes(p), aiCertBottomLine(p), p.File, aiCertPresetLink, p.File)
	rows := append([]aiCertPresetTask(nil), p.Tasks...)
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].Label < rows[j].Label })
	page.WriteString("| Feature | Can I use it? | In plain words |\n|---|---|---|\n")
	for _, row := range rows {
		fmt.Fprintf(page, "| %s <sub>`%s`</sub> | %s | %s |\n",
			row.Label, row.Task, aiCertGrade(row), aiCertPlainWords(row))
	}
	page.WriteString("\n<details>\n<summary>Which models this preset uses</summary>\n\n")
	page.WriteString("| Tier | Provider | Model |\n|---|---|---|\n")
	for _, tier := range p.Tiers {
		fmt.Fprintf(page, "| `%s` | `%s` | `%s` |\n", tier.Tier, tier.Provider, tier.Model)
	}
	page.WriteString("\nEach feature walks its own ladder of tiers until it reaches one this preset\n")
	page.WriteString("binds; this is the rung and the model it lands on, and the record behind its grade.\n\n")
	page.WriteString("| Task | Served on | Model | Band | State |\n|---|---|---|---|---|\n")
	for _, row := range p.Tasks {
		fmt.Fprintf(page, "| `%s` | %s | %s | %s | %s |\n",
			row.Task, aiCertCell(row.Tier), aiCertCell(row.Model.Model),
			aiCertBandCell(row), aiCertCell(row.State))
	}
	page.WriteString("\n</details>\n\n")
}

// aiCertGrade is the answer to "can I use it?" for one feature. Off and not
// measured are different answers: the first is the preset's choice, the
// second a gap in the testing.
func aiCertGrade(row aiCertPresetTask) string {
	switch {
	case row.Tier == "":
		return aiCertOff
	case row.Band == "":
		return aiCertUnproven
	case row.Band == aicert.VerdictCertified:
		return aiCertReady
	case row.Band == aicert.VerdictSupportedDegraded:
		return aiCertCare
	case row.Band == aicert.VerdictNotSupported:
		return aiCertNotYet
	default:
		return "`" + row.Band + "`"
	}
}

// aiCertPlainWords says what the grade rests on: how often the answer was
// right, which half of the rule held it down, and whether it is still the
// product as it ships.
func aiCertPlainWords(row aiCertPresetTask) string {
	switch {
	case row.Tier == "":
		return "Off — this preset has no model for it"
	case row.Band == "":
		return "Not measured yet"
	}
	words := aiCertTriesWords(row.Runs, row.Passed)
	if row.Band != aicert.VerdictCertified {
		if row.CasesFailingOften > 0 {
			words += "; " + aiCertCases(row.CasesFailingOften) + " wrong too often"
		}
		if row.CasesBelowQualityBar > 0 {
			words += "; answer quality below the bar in " + aiCertCases(row.CasesBelowQualityBar)
		}
	}
	switch row.State {
	case aicert.StatusStale:
		words += " · re-check pending"
	case aicert.StatusPartial:
		words += " · newer test cases not tried yet"
	}
	return words
}

func aiCertTriesWords(runs, passed int) string {
	switch {
	case runs == 0:
		return "No tries recorded"
	case passed == runs:
		return fmt.Sprintf("Right every time (%d of %d)", passed, runs)
	default:
		return fmt.Sprintf("Right in %d of %d tries", passed, runs)
	}
}

func aiCertCases(n int) string {
	if n == 1 {
		return "one test case"
	}
	return fmt.Sprintf("%d test cases", n)
}

// writeAICertGrading explains the grades in words, then states the exact rule
// for whoever needs to argue with one. The numbers in the words are the rule's
// own: the pass rate is its constant, the try count the runner's default.
func writeAICertGrading(page *strings.Builder, rule string, selfJudged int, bars []aiCertQualityBar) {
	page.WriteString("## How the scoring works\n\n")
	page.WriteString("1. **Real test cases.** Every feature has a set of test cases: a realistic\n")
	page.WriteString("   situation (an email, an account, a web page) and the answer we expect. The\n")
	page.WriteString("   model receives exactly the prompt the product sends in real use.\n")
	fmt.Fprintf(page, "2. **Several tries.** Each test case is run %d times (our standard setting; `RUNS=`\n"+
		"   can change it for one run), because a model can answer the same question\n"+
		"   differently each time.\n", aicert.DefaultRepeats)
	page.WriteString("3. **Two checks on every try.**\n")
	page.WriteString("   - *Is it right?* The answer is checked mechanically against what we expect: the\n")
	page.WriteString("     right label, the right record, no invented facts, fast enough.\n")
	page.WriteString("   - *Is it good?* A second AI model, chosen so that it is not the one being tested,\n")
	page.WriteString("     scores the answer from 0 to 100 against a written description of a good answer.\n")
	if selfJudged > 0 {
		fmt.Fprintf(page, "     %d older results were scored by the same model they tested; the next re-check\n"+
			"     replaces them.\n", selfJudged)
	}
	page.WriteString("4. **A low score is double-checked.** When the quality score is below the bar, the\n")
	fmt.Fprintf(page, "   scoring model is asked %d more times and the middle of the %d scores counts, so\n"+
		"   one bad reading cannot fail a good answer.\n", aicert.RejudgeOpinions, aicert.RejudgeOpinions+1)
	page.WriteString("5. **The grade.** All the tries of a feature are then added up:\n\n")
	page.WriteString("| Grade | Right answers | Every test case | Quality |\n|---|---|---|---|\n")
	fmt.Fprintf(page, "| %s | at least %d of every 100 tries | right in at least %d of its %d tries | good in every case, no very poor answer |\n",
		aiCertReady, aicert.CertifiedPassPercent, aicert.CaseMajority(aicert.DefaultRepeats), aicert.DefaultRepeats)
	fmt.Fprintf(page, "| %s | at least %s of all tries | — | acceptable in every case |\n", aiCertCare, aiCertMajorityWords())
	fmt.Fprintf(page, "| %s | anything less | | |\n\n", aiCertNotYet)
	page.WriteString("A feature does not have to be perfect to be ready: a stray miss among many tries\n")
	page.WriteString("is allowed. A test case that fails again and again is not — that is a real\n")
	page.WriteString("weakness, not bad luck — and it holds the whole feature back.\n\n")
	writeAICertThresholds(page, bars)
	page.WriteString("<details>\n<summary>The exact rule</summary>\n\n")
	page.WriteString("From `Verdict` in [`" + aiCertVerdictSource + "`](" + corpusLinkPrefix + aiCertCorpusDocs +
		aiCertVerdictSource + "), applied to every case of a task at once:\n\n")
	page.WriteString("```text\n" + rule + "\n```\n\n")
	page.WriteString("Each case sets its own quality bands (`certified_min`, `degraded_min`, `floor`).\n")
	fmt.Fprintf(page, "A judge score below `certified_min` is asked for %d more times, and the run is\n"+
		"scored at the median of the %d. The grades map to the record's words as\n", aicert.RejudgeOpinions, aicert.RejudgeOpinions+1)
	fmt.Fprintf(page, "%s = `%s`, %s = `%s`, %s = `%s`, and %s = no record for that model.\n",
		aiCertReady, aicert.VerdictCertified, aiCertCare, aicert.VerdictSupportedDegraded,
		aiCertNotYet, aicert.VerdictNotSupported, aiCertUnproven)
	if selfJudged > 0 {
		fmt.Fprintf(page, "\n%d of the committed records were nonetheless graded by the model they measured\n"+
			"(`self_judged` in the record file).\n", selfJudged)
	}
	page.WriteString("\n</details>\n\n")
}

// aiCertQualityBar is one set of quality bands and how many cases of the corpus
// are graded against it.
type aiCertQualityBar struct {
	Bands aicert.Bands
	Cases int
}

// aiCertQualityBars groups the corpus by its bands, the most used first, so the
// page states every bar a case is held to without listing each case.
func aiCertQualityBars(corpus []aicert.Scenario) []aiCertQualityBar {
	counts := map[aicert.Bands]int{}
	for _, sc := range corpus {
		counts[sc.Expect.Bands]++
	}
	bars := make([]aiCertQualityBar, 0, len(counts))
	for bands, cases := range counts {
		bars = append(bars, aiCertQualityBar{Bands: bands, Cases: cases})
	}
	sort.Slice(bars, func(i, j int) bool {
		if bars[i].Cases != bars[j].Cases {
			return bars[i].Cases > bars[j].Cases
		}
		a, b := bars[i].Bands, bars[j].Bands
		if a.CertifiedMin != b.CertifiedMin {
			return a.CertifiedMin > b.CertifiedMin
		}
		if a.DegradedMin != b.DegradedMin {
			return a.DegradedMin > b.DegradedMin
		}
		return a.Floor > b.Floor
	})
	return bars
}

// aiCertThresholdsHeading opens the table of every number a grade is reached by.
const aiCertThresholdsHeading = "### Thresholds\n\n"

// writeAICertThresholds states every threshold in plain words, each read from
// the constant or the corpus that sets it.
func writeAICertThresholds(page *strings.Builder, bars []aiCertQualityBar) {
	page.WriteString(aiCertThresholdsHeading)
	page.WriteString("| What | Threshold |\n|---|---|\n")
	fmt.Fprintf(page, "| Right answers needed for %s | at least %d of every 100 tries, counted over all of the feature's test cases |\n",
		aiCertReady, aicert.CertifiedPassPercent)
	fmt.Fprintf(page, "| Each test case, for %s | right in at least %d of its %d tries |\n",
		aiCertReady, aicert.CaseMajority(aicert.DefaultRepeats), aicert.DefaultRepeats)
	fmt.Fprintf(page, "| Right answers needed for %s | at least %s of all tries |\n", aiCertCare, aiCertMajorityWords())
	fmt.Fprintf(page, "| Tries per test case | %d (`RUNS=` changes it for one run) |\n", aicert.DefaultRepeats)
	fmt.Fprintf(page, "| Extra quality opinions on a low score | %d more, and the middle of the %d scores counts |\n",
		aicert.RejudgeOpinions, aicert.RejudgeOpinions+1)
	for _, bar := range bars {
		b := bar.Bands
		fmt.Fprintf(page, "| Quality bar %d / %d / %d — %s | %s needs a quality score of at least %d (and no single answer below %d); %s needs at least %d |\n",
			b.CertifiedMin, b.DegradedMin, b.Floor, aiCertCases(bar.Cases),
			aiCertReady, b.CertifiedMin, b.Floor, aiCertCare, b.DegradedMin)
	}
	page.WriteString("\nAll of these live in [`backend/internal/compose/aicert/thresholds.go`](" + corpusLinkPrefix + aiCertCorpusDocs +
		"thresholds.go) (quality bars: in each test case's file); change them there and regenerate this page.\n\n")
}

// assertAICertThresholdsStateTheRule holds the Thresholds table to the constants
// and the corpus, so a number typed into its writer by hand fails here.
func assertAICertThresholdsStateTheRule(t *testing.T, page string, corpus []aicert.Scenario) {
	t.Helper()
	_, table, found := strings.Cut(page, aiCertThresholdsHeading)
	if !found {
		t.Fatalf("the page has no %q table", strings.TrimSpace(aiCertThresholdsHeading))
	}
	table, _, _ = strings.Cut(table, "\n\n")
	want := []string{
		fmt.Sprintf("| Right answers needed for %s | at least %d of every 100 tries", aiCertReady, aicert.CertifiedPassPercent),
		fmt.Sprintf("| Tries per test case | %d ", aicert.DefaultRepeats),
	}
	for _, sc := range corpus {
		b := sc.Expect.Bands
		want = append(want, fmt.Sprintf("| Quality bar %d / %d / %d — ", b.CertifiedMin, b.DegradedMin, b.Floor))
	}
	for _, row := range want {
		if !strings.Contains(table, row) {
			t.Errorf("the Thresholds table does not state %q:\n%s", row, table)
		}
	}
}

// aiCertMajorityWords says the case majority as a fraction in words, "two
// thirds", falling back to digits for a fraction English has no short name for.
func aiCertMajorityWords() string {
	num, den := aicert.MajorityNumerator, aicert.MajorityDenominator
	counts := []string{"", "one", "two", "three", "four"}
	parts := map[int]string{2: "half", 3: "third", 4: "quarter", 5: "fifth"}
	part, named := parts[den]
	if num < 1 || num >= len(counts) || !named {
		return fmt.Sprintf("%d/%d", num, den)
	}
	if num > 1 {
		part += "s"
	}
	return counts[num] + " " + part
}

// aiCertCell renders an unmeasured or unbound value as the page's own dash
// rather than as an empty table cell, which reads as a rendering fault.
func aiCertCell(value string) string {
	if value == "" {
		return "-"
	}
	return "`" + value + "`"
}

func aiCertBandCell(row aiCertPresetTask) string {
	if row.Tier == "" {
		return "`unbound`"
	}
	if row.Band == "" {
		return "`untested`"
	}
	return "`" + row.Band + "`"
}

// A preset's bottom line counts ready features from whatever grades it has, so it
// says when those grades are waiting on a re-check — all of them, or how many.
func TestAPresetsBottomLineSaysHowManyGradesArePending(t *testing.T) {
	row := func(band, state string) aiCertPresetTask {
		return aiCertPresetTask{Tier: "premium", Band: band, State: state}
	}
	current, stale := aicert.StatusCurrent, aicert.StatusStale
	for _, tc := range []struct {
		name  string
		tasks []aiCertPresetTask
		want  string
	}{
		{"every grade current", []aiCertPresetTask{row(aicert.VerdictCertified, current), row("", "")}, "1 of 2 features ready"},
		{"every grade stale", []aiCertPresetTask{row(aicert.VerdictCertified, stale), row("", "")}, "1 of 2 features ready (re-check pending)"},
		{"one grade stale", []aiCertPresetTask{row(aicert.VerdictCertified, stale), row(aicert.VerdictNotSupported, current)}, "1 of 2 features ready (1 re-check pending)"},
		{"some grades stale", []aiCertPresetTask{
			row(aicert.VerdictCertified, stale), row(aicert.VerdictNotSupported, stale), row(aicert.VerdictCertified, current),
		}, "2 of 3 features ready (2 re-checks pending)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			preset := aiCertPreset{Tasks: tc.tasks}
			for _, r := range tc.tasks {
				if r.Band == aicert.VerdictCertified {
					preset.Bands.Certified++
				}
			}
			if got := aiCertBottomLine(preset); got != tc.want {
				t.Errorf("bottom line = %q, want %q", got, tc.want)
			}
		})
	}
}
