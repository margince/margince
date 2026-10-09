// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The top of docs/reference/mcp-tool-coverage.md, written for someone choosing
// an assistant rather than someone changing the lane: which assistant to use,
// and which everyday jobs each one does reliably. The engineering detail the
// rest of the page carries is unchanged beneath it.

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// routeOf names how the assistant was reached. A folder's @suffix is the CLI
// route; a verdict with no driver predates routes, when only the claude CLI drove.
func routeOf(folder, driver string) string {
	switch {
	case strings.HasSuffix(folder, "@codex-cli"):
		return "Codex CLI"
	case strings.HasSuffix(folder, "@claude-cli"), driver == "":
		return "Claude Code CLI"
	case strings.HasSuffix(driver, ":openrouter"):
		return "neutral harness, via OpenRouter"
	default:
		return "neutral harness, vendor API"
	}
}

func modelName(folder string) string {
	name, _, _ := strings.Cut(folder, "@")
	return name
}

// trustVerdict mirrors ai-certification.md's family verdict: every job held is
// Yes, at least 80 in 100 is Mostly, fewer is Not yet, and under half the jobs
// tried says nothing at all.
func trustVerdict(held, tried, cases int) string {
	switch {
	case tried*2 < cases:
		return "⚪ Not enough tested yet"
	case held == tried:
		return "🟢 Yes"
	case held*100 >= tried*80:
		return "🟡 Mostly"
	default:
		return "🔴 Not yet"
	}
}

func writeCoverageIntro(p *strings.Builder, r mcpToolCoverage) {
	p.WriteString("Which AI assistant can do which everyday job in Margince (logging a call, preparing " +
		"for a meeting, tidying up records) once it is connected to Margince. Every result was " +
		"measured by having the real assistant do the job several times on a test company and " +
		"checking what it did and what it said.\n\n")
	p.WriteString("<details>\n<summary>How this page is made</summary>\n\n")
	p.WriteString(r.Note + "\n\n")
	p.WriteString("The lane that produces its results is `make e2e-llm`; it is paid and opt-in, so a " +
		"result changes only when someone runs it and commits the verdict. For whether a model can " +
		"do one AI feature, see [ai-certification.md](ai-certification.md).\n\n</details>\n\n")
}

func writeCoverageAssistants(p *strings.Builder, r mcpToolCoverage) {
	p.WriteString("## Which assistant should I use?\n\n")
	if len(r.Models) == 0 {
		p.WriteString("No assistant has been measured yet.\n\n")
		return
	}
	titles := caseTitles(r.Cases)
	p.WriteString("| Assistant | How we connected it | Jobs tried | Can I trust it? | In plain words |\n")
	p.WriteString("|---|---|---:|---|---|\n")
	for _, m := range r.Models {
		fmt.Fprintf(p, "| `%s` | %s | %d of %d | %s | %s |\n", modelName(m.Model), m.Route,
			m.CasesRecorded, len(r.Cases), trustVerdict(m.CasesHeld, m.CasesRecorded, len(r.Cases)),
			plainWords(m, titles))
	}
	p.WriteString("\nAn assistant rated **Yes** was reliable on every job we tried. One rated **Mostly** was " +
		"reliable on at least 80 in every 100, and one rated **Not yet** on fewer. We say **Not enough " +
		"tested yet** when we tried it on under half the jobs.\n\n*Reliably* means it got the job right in most of its tries: " +
		"each job is tried several times, because an assistant does not answer the same way twice.\n\n")
	p.WriteString("**How we connected it** matters. *Claude Code CLI* and *Codex CLI* are each vendor's own " +
		"assistant, with its own instructions. The *neutral harness* gives every model the same " +
		"tools and the same instructions, so it is the only route that compares one model with another.\n\n")
}

func plainWords(m modelCoverage, titles map[string]string) string {
	jobs := "jobs"
	if m.CasesRecorded == 1 {
		jobs = "job"
	}
	held := fmt.Sprintf("Reliable on %d of %d %s tried", m.CasesHeld, m.CasesRecorded, jobs)
	if len(m.BelowBar) > 0 {
		names := make([]string, 0, len(m.BelowBar))
		for _, name := range m.BelowBar {
			names = append(names, titles[name])
		}
		held += "; not yet: " + strings.Join(names, ", ")
	}
	if m.Search == "lexical" {
		held += ". Measured with search working on word matches only, which is harder than a real setup"
	}
	return held + "."
}

func writeCoverageJobs(p *strings.Builder, r mcpToolCoverage) {
	p.WriteString("## What can it do for me?\n\n")
	p.WriteString("One row per everyday job, one column per assistant. ✅ did it reliably, ❌ not reliably " +
		"yet, and a dash means not tried yet. What a good answer must do is in the second column.\n\n")
	header := "| Job | What a good answer does |"
	rule := "|---|---|"
	var headerSb115 strings.Builder
	var ruleSb115 strings.Builder
	for _, m := range r.Models {
		fmt.Fprintf(&headerSb115, " `%s`<br>%s |", modelName(m.Model), m.Route)
		ruleSb115.WriteString(":---:|")
	}
	header += headerSb115.String()
	rule += ruleSb115.String()
	p.WriteString(header + "\n" + rule + "\n")
	for _, c := range byCaseNumber(r.Cases) {
		fmt.Fprintf(p, "| %s | %s |", c.Title, plainCriteria(c.Name, c.Criteria, r.Criteria))
		for _, m := range r.Models {
			p.WriteString(" " + jobMark(c, m.Model) + " |")
		}
		p.WriteString("\n")
	}
	p.WriteString("\n")
}

// plainCriteria is criteriaNames without the numbers, which only an engineer
// cross-referencing the scenario file needs.
func plainCriteria(caseName string, numbers []int, catalog []criterionRow) string {
	named := criterionNames(caseName, catalog)
	out := make([]string, 0, len(numbers))
	for _, n := range numbers {
		if name, ok := named[n]; ok {
			out = append(out, name)
		}
	}
	if len(out) == 0 {
		return "-"
	}
	return strings.Join(out, "<br>")
}

func jobMark(c caseRow, model string) string {
	for _, run := range c.ByModel {
		if run.Model == model {
			if run.Held {
				return "✅"
			}
			return "❌"
		}
	}
	return "-"
}

func caseTitles(cases []caseRow) map[string]string {
	out := make(map[string]string, len(cases))
	for _, c := range cases {
		out[c.Name] = c.Title
	}
	return out
}

var caseNumber = regexp.MustCompile(`^case(\d+)`)

// byCaseNumber orders jobs as their authors numbered them; the engineering
// tables keep file-name order, where case10 sorts before case2.
func byCaseNumber(cases []caseRow) []caseRow {
	out := append([]caseRow(nil), cases...)
	number := func(c caseRow) int {
		if got := caseNumber.FindStringSubmatch(c.Name); len(got) == 2 {
			n, err := strconv.Atoi(got[1])
			if err == nil {
				return n
			}
		}
		return 0
	}
	sort.SliceStable(out, func(i, j int) bool { return number(out[i]) < number(out[j]) })
	return out
}
