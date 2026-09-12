// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// docs/reference/mcp-tool-coverage.md as a reader meets it: the prose half of
// the page whose data half is assembled in mcptoolcoverage_test.go.
//
// Split by concept rather than by size. Reading the lane — which scenarios
// exist, which tools they require, what each model's runs came to — is a
// different subject from deciding what a contact needs to be told first, and the
// two together carried the one file past its ceiling.

import (
	"fmt"
	"strings"
)

func renderMCPToolCoveragePage(r mcpToolCoverage) []byte {
	var p strings.Builder
	p.WriteString("# What the assistant can be relied on to do\n\n")
	p.WriteString("<!-- Generated together with mcp-tool-coverage.json; do not edit by hand. -->\n\n")
	p.WriteString(r.Note + "\n\n")
	p.WriteString("**This page is generated, and an edit made here is lost.**\n\n")
	writeCoverageHowToRead(&p)
	writeCoverageTotals(&p, r)
	writeCoverageSurfaces(&p, r)
	writeCoverageCases(&p, r)
	writeCoverageCriteria(&p, r)
	writeCoverageReliable(&p, r)
	writeCoverageFailing(&p, r)
	writeCoverageJudges(&p, r)
	writeCoverageOneOfASet(&p, r)
	writeCoverageUndriven(&p, r)
	return []byte(p.String())
}

// writeCoverageHowToRead says what a row means before any row appears. "Driven"
// in particular is not what it sounds like, and a reader meeting a 0 in the last
// section should not have to guess whether it means broken or untried.
func writeCoverageHowToRead(p *strings.Builder) {
	p.WriteString("## How to read this page\n\n")
	p.WriteString("| Word | What it means |\n|---|---|\n")
	p.WriteString("| Tool | One action the assistant can take, such as `send_email` or `run_report`. |\n")
	p.WriteString("| Case | One use case, driven end to end by a real assistant against a seeded " +
		"installation — the shape a user meets, not a single step. |\n")
	p.WriteString("| Requires | The case FAILS if the assistant never calls this tool. This is what " +
		"coverage means here. |\n")
	p.WriteString("| Permitted | The case allows the tool without needing it. A case can pass having " +
		"never touched a tool it permits, so **permission is not coverage**. |\n")
	p.WriteString("| Required as one of a set | Some questions have more than one honest engine, so " +
		"a case can require `A` **or** `B` and fail if neither is called. Every run reaches for one " +
		"of the set, and any one member can go uncalled — which is neither of the two words above. |\n")
	p.WriteString("| Driven | At least one case requires this tool. A tool no case requires is " +
		"**untried, not broken** — nothing has ever asked an assistant to reach for it. |\n")
	p.WriteString("| Reliability | Of the runs on the cases that require this tool, the share that " +
		"passed. Each case is run several times because the lane is not deterministic. |\n")
	p.WriteString("| Bar | Each case carries its own `pass_at` — how many of its runs must pass. " +
		"Cases are not equally hard, so one shared number would flatter the easy ones. |\n\n")
	p.WriteString("This page does not grade single steps or name a best model per site — that is\n")
	p.WriteString("[ai-certification.md](ai-certification.md), a different lane asking a different " +
		"question. Read it beside this one.\n\n")
}

func writeCoverageTotals(p *strings.Builder, r mcpToolCoverage) {
	p.WriteString("## The short version\n\n")
	fmt.Fprintf(p, "| | |\n|---|---:|\n")
	fmt.Fprintf(p, "| Tools the assistant is offered | %d |\n", r.Totals.Tools)
	fmt.Fprintf(p, "| … some case requires | %d |\n", r.Totals.Driven)
	fmt.Fprintf(p, "| … some case requires as one of a set | %d |\n", r.Totals.OneOfASet)
	fmt.Fprintf(p, "| … **no case requires** | %d |\n", r.Totals.NeverDriven)
	fmt.Fprintf(p, "| … of those, permitted somewhere but never required | %d |\n", r.Totals.PermittedNotDriven)
	fmt.Fprintf(p, "| Prompt tokens spent on tools no case requires | %d |\n", r.Totals.UndrivenCost)
	fmt.Fprintf(p, "| Use cases | %d |\n", r.Totals.Cases)
	fmt.Fprintf(p, "| Acceptance criteria the cases declare, each with a statement | %d |\n\n", r.Totals.CriteriaNamed)

	if len(r.Models) == 0 {
		p.WriteString("> **No model has a committed run.** Every case below says `not run` rather " +
			"than a rate — nobody has paid for the answer yet, and an empty lane is not a failing one.\n\n")
		return
	}

	p.WriteString("## By model\n\n")
	p.WriteString("Which model drove the lane, and how it went. The tool columns further down are " +
		"the same for every model — what a case REQUIRES is the scenario's property, not the " +
		"driver's — so what changes here is whether the driving succeeded.\n\n")
	p.WriteString("| Model | Cases run | Reached their bar | Below it | Runs passed | Reliability |\n" +
		"|---|---:|---:|---:|---:|---:|\n")
	for _, m := range r.Models {
		fmt.Fprintf(p, "| `%s` | %d of %d | %d | %d | %d/%d | %.0f%% |\n",
			m.Model, m.CasesRecorded, r.Totals.Cases, m.CasesHeld, m.CasesBelowBar,
			m.Passed, m.Runs, 100*m.Reliability)
	}
	p.WriteString("\n")
	for _, m := range r.Models {
		if m.CasesRecorded < r.Totals.Cases {
			fmt.Fprintf(p, "> `%s` has no committed run for %d of %d cases.\n\n",
				m.Model, r.Totals.Cases-m.CasesRecorded, r.Totals.Cases)
		}
		if len(m.BelowBar) > 0 {
			fmt.Fprintf(p, "> `%s` below its bar on: %s\n\n", m.Model, strings.Join(m.BelowBar, ", "))
		}
	}
}

// writeCoverageCases is the lane itself, case by case: what each one is for and
// how it went. A manager reading only one table should read this one.
func writeCoverageCases(p *strings.Builder, r mcpToolCoverage) {
	p.WriteString("## The use cases\n\n")
	p.WriteString("One row per case per model that ran it. A case nobody has run appears once, " +
		"marked `not run`, rather than being dropped — a case missing from this table would read " +
		"as a case that does not exist.\n\n")
	p.WriteString("| Case | Model | Result | Passed | Bar | Criteria | Requires |\n" +
		"|---|---|---|---:|---:|---|---|\n")
	for _, c := range r.Cases {
		criteria := criteriaNames(c.Name, c.Criteria, r.Criteria)
		link := fmt.Sprintf("[%s](../../e2e/llm/scenarios/%s)", c.Name, c.File)
		if len(c.ByModel) == 0 {
			fmt.Fprintf(p, "| %s | — | not run | — | — | %s | %s |\n",
				link, criteria, requiresColumn(c))
			continue
		}
		for _, run := range c.ByModel {
			result := "**FAIL**"
			if run.Held {
				result = "pass"
			}
			fmt.Fprintf(p, "| %s | `%s` | %s | %d/%d | %d | %s | %s |\n",
				link, run.Model, result, run.Passed, run.Runs, run.PassAt,
				criteria, requiresColumn(c))
		}
	}
	p.WriteString("\n")
}

// writeCoverageCriteria states what each number a case declares actually asks.
// Without it the case table grades against numbers a reader cannot look up.
func writeCoverageCriteria(p *strings.Builder, r mcpToolCoverage) {
	p.WriteString("## What the criteria ask\n\n")
	p.WriteString("The numbers in the table above, in words. Source: " +
		"[`e2e/llm/criteria.yaml`](../../e2e/llm/criteria.yaml).\n\n")
	p.WriteString("**The numbers are per case.** Case 1 criterion 1 and case 2 criterion 1 are " +
		"different criteria that share a digit, which is why every row below names its case.\n\n")
	p.WriteString("| Case | # | Criterion | What it asks |\n|---|---:|---|---|\n")
	for _, c := range r.Criteria {
		fmt.Fprintf(p, "| `%s` | %d | **%s** | %s |\n", c.Case, c.Number, c.Name, c.Statement)
	}
	p.WriteString("\n")
}

// writeCoverageReliable is the first question: what can I put in front of
// somebody. Only a tool whose every covering case cleared its own bar with a
// clean rate qualifies — PER MODEL, because "reliable" is a property of the
// tool and the model together. A tool one model drives perfectly and another
// fumbles is not a reliable tool; it is a reliable pair, and a page that folded
// the two would recommend a combination nobody measured.
func writeCoverageReliable(p *strings.Builder, r mcpToolCoverage) {
	p.WriteString("## 1. What you can rely on\n\n")
	if len(r.Models) == 0 {
		p.WriteString("_No model has a committed run._\n\n")
		return
	}
	p.WriteString("Every run of every case requiring this tool passed, for the model named.\n\n")
	for _, m := range r.Models {
		fmt.Fprintf(p, "### `%s`\n\n", m.Model)
		p.WriteString("| Tool | Reliability | Runs | Required by |\n|---|---:|---:|---|\n")
		rows := 0
		for _, row := range r.Tools {
			measured := row.MeasuredByModel[m.Model]
			if measured == nil || measured.Reliability < 1 || len(measured.BelowBar) > 0 {
				continue
			}
			rows++
			fmt.Fprintf(p, "| `%s` | %.2f | %d | %s |\n",
				row.Name, measured.Reliability, measured.Runs, joinOrDash(row.MustCall))
		}
		if rows == 0 {
			p.WriteString("| _nothing yet_ | - | - | - |\n")
		}
		p.WriteString("\n")
	}
}

// writeCoverageFailing is the second question: what is wrong today. A tool
// appears here when it was driven and a run did not pass — including a case
// that cleared its bar while losing a run, which is a rate worth seeing.
func writeCoverageFailing(p *strings.Builder, r mcpToolCoverage) {
	p.WriteString("## 2. What is failing now\n\n")
	if len(r.Models) == 0 {
		p.WriteString("_No model has a committed run._\n\n")
		return
	}
	p.WriteString("Driven, and not every run passed. Open the case to see what was asked.\n\n")
	for _, m := range r.Models {
		fmt.Fprintf(p, "### `%s`\n\n", m.Model)
		p.WriteString("| Tool | Reliability | Passed | Below its bar | Required by |\n" +
			"|---|---:|---:|---|---|\n")
		rows := 0
		for _, row := range r.Tools {
			measured := row.MeasuredByModel[m.Model]
			if measured == nil || (measured.Reliability >= 1 && len(measured.BelowBar) == 0) {
				continue
			}
			rows++
			fmt.Fprintf(p, "| `%s` | %.2f | %d/%d | %s | %s |\n", row.Name, measured.Reliability,
				measured.Passed, measured.Runs, joinOrDash(measured.BelowBar), joinOrDash(row.MustCall))
		}
		if rows == 0 {
			p.WriteString("| _nothing driven is failing_ | - | - | - | - |\n")
		}
		p.WriteString("\n")
	}
}

// writeCoverageUndriven is the last question, and the one this page exists
// for: what has nobody written a case for. Ordered by what each costs, because
// that is the bill being paid for the untried thing.
func writeCoverageUndriven(p *strings.Builder, r mcpToolCoverage) {
	p.WriteString("## 4. What no use case requires\n\n")
	p.WriteString("**Untried by THIS lane, which is not the same as ungraded.** The `Graded by` " +
		"column names the certification tasks whose\n")
	p.WriteString("corpus tests the tool anyway — that lane asks which tool a goal should reach " +
		"for, and which plausible neighbour it must\n")
	p.WriteString("avoid, which this lane cannot express at all: it sees that a name appeared, " +
		"never whether it was the right first reach.\n\n")
	fmt.Fprintf(p, "So of the %d tools no use case requires, **%d are graded elsewhere** and %d are "+
		"untried by any lane.\n\n",
		r.Totals.NeverDriven, r.Totals.UndrivenButGraded, r.Totals.NeverDriven-r.Totals.UndrivenButGraded)
	p.WriteString("A tool in the `Permitted in` column is worse than one with nothing: a case is " +
		"allowed to use it and no case checks that it can.\n\n")
	p.WriteString("| Tool | Tokens | Graded by | Permitted in | Attached to |\n|---|---:|---|---|---|\n")
	rows := 0
	for _, row := range r.Tools {
		// A tool some case requires as one of a set has its own section above:
		// listing it here too would print it as untried on the page that just
		// said a case reaches for it.
		if row.Driven || row.OneOfASet {
			continue
		}
		rows++
		fmt.Fprintf(p, "| `%s` | %d | %s | %s | %s |\n",
			row.Name, row.Tokens, joinOrDash(row.GradedBy), joinOrDash(row.MayCall), joinOrDash(row.Agents))
	}
	if rows == 0 {
		p.WriteString("| _every tool has a case_ | - | - | - | - |\n")
	}
	p.WriteString("\n")
}

// criteriaNames renders a case's criteria as "8 promises come back as
// suggestions" rather than "8" — a number is only a reference, and the table it
// referenced was not in this repository.
func criteriaNames(caseName string, numbers []int, catalog []criterionRow) string {
	if len(numbers) == 0 {
		return "—"
	}
	named := map[int]string{}
	for _, c := range catalog {
		if c.Case == caseName {
			named[c.Number] = c.Name
		}
	}
	out := make([]string, 0, len(numbers))
	for _, n := range numbers {
		if name, ok := named[n]; ok {
			out = append(out, fmt.Sprintf("**%d** %s", n, name))
			continue
		}
		out = append(out, fmt.Sprintf("**%d**", n))
	}
	return strings.Join(out, "<br>")
}

// requiresColumn renders what a case demands, with an any-of group written as
// the alternatives it is — "`a` or `b`" — rather than as two more names in a
// comma list. A reader of that list would take each member for something the
// case fails without, which is exactly what the group does not say.
func requiresColumn(c caseRow) string {
	parts := make([]string, 0, len(c.Requires)+len(c.RequiresOneOf))
	for _, name := range c.Requires {
		parts = append(parts, "`"+name+"`")
	}
	for _, group := range c.RequiresOneOf {
		parts = append(parts, strings.Join(backticked(group), " or "))
	}
	if len(parts) == 0 {
		return "—"
	}
	return strings.Join(parts, ", ")
}

// writeCoverageOneOfASet is the state between the two the page had: tools no
// case requires on their own, which every run of some case nevertheless reaches
// for one of.
//
// They are kept out of the untried bill below on purpose. A tool here is not
// work nobody has done — a case drives the set — but it is not something to put
// in front of somebody either, because no run proves this member was the one
// called. Neither existing section can say that, which is why this one exists.
func writeCoverageOneOfASet(p *strings.Builder, r mcpToolCoverage) {
	p.WriteString("## 3. What a case requires only as one of a set\n\n")
	p.WriteString("The case fails if NONE of the set is called, and passes having called any one of " +
		"them. So a run may never touch the tool named here,\n")
	p.WriteString("and the case's pass rate belongs to the set rather than to this row — which is " +
		"why no reliability is printed for it above.\n\n")
	p.WriteString("| Tool | Tokens | Required as one of a set by | Permitted in | Graded by | Attached to |\n" +
		"|---|---:|---|---|---|---|\n")
	rows := 0
	for _, row := range r.Tools {
		if !row.OneOfASet {
			continue
		}
		rows++
		fmt.Fprintf(p, "| `%s` | %d | %s | %s | %s | %s |\n",
			row.Name, row.Tokens, joinOrDash(row.MustCallOneOf), joinOrDash(row.MayCall),
			joinOrDash(row.GradedBy), joinOrDash(row.Agents))
	}
	if rows == 0 {
		p.WriteString("| _no case offers a choice of tools_ | - | - | - | - | - |\n")
	}
	p.WriteString("\n")
}

// backticked quotes each name for a markdown cell.
func backticked(in []string) []string {
	out := make([]string, 0, len(in))
	for _, v := range in {
		out = append(out, "`"+v+"`")
	}
	return out
}

// joinOrDash renders a list as backticked names, or a dash when it is empty.
func joinOrDash(in []string) string {
	if len(in) == 0 {
		return "—"
	}
	return strings.Join(backticked(in), ", ")
}
