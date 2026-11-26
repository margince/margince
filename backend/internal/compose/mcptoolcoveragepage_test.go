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
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/mcpe2e"
)

func renderMCPToolCoveragePage(r mcpToolCoverage) []byte {
	var p strings.Builder
	p.WriteString("# Your AI assistant: test results\n\n")
	p.WriteString("<!-- Generated together with mcp-tool-coverage.json; do not edit by hand. -->\n\n")
	p.WriteString("Which AI you can use, both ways, is on one page: [Which AI can I use with Margince?](which-ai-you-can-use.md). " +
		"The results for the AI inside the product are in [Margince AI: test results](ai-certification.md).\n\n")
	writeCoverageIntro(&p, r)
	writeCoverageAssistants(&p, r)
	writeCoverageJobs(&p, r)
	writeCoverageNeutral(&p, r)
	writeCoverageREST(&p, r)
	p.WriteString("---\n\n## For engineers\n\nEverything below is the detail behind the two tables above: " +
		"which tools each job requires, every run's numbers, and what no job tests yet.\n\n")
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
		"installation: the shape a user meets, rather than a single step. |\n")
	p.WriteString("| Requires | The case fails if the assistant never calls this tool. This is what " +
		"coverage means here. |\n")
	p.WriteString("| Permitted | The case allows the tool without needing it. A case can pass having " +
		"never touched a tool it permits, so a permitted tool is not covered by that case. |\n")
	p.WriteString("| Required as one of a set | Some questions can be answered by more than one tool, so " +
		"a case can require `A` or `B` and fails if neither is called. Every run reaches for one " +
		"of the set, and any one member can go uncalled, so this is neither of the two words above. |\n")
	p.WriteString("| Driven | At least one case requires this tool, or a passing run of a case that held called it. " +
		"A tool neither is untried: nothing says it is broken. |\n")
	p.WriteString("| Reliability | Of the runs on the cases that require this tool, the share that " +
		"passed. Each case is run several times because the lane is not deterministic. |\n")
	p.WriteString("| Bar | Each case carries its own `pass_at`: how many of its runs must pass. " +
		"Cases are not equally hard, so one shared number would flatter the easy ones. |\n")
	p.WriteString("| Calls, turns | How directly the runs finished: the median tool calls and model turns, " +
		"with their range. `slow` marks a case past its `expected_calls`. A score, never a fail. |\n")
	p.WriteString("| Re-graded | A later checker graded the saved transcripts of an earlier run. The model ran " +
		"once, at the run's commit, and the verdict names both commits. |\n\n")
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
			"than a rate: nobody has paid for the answer yet, and an empty lane is not a failing one.\n\n")
		return
	}

	p.WriteString("## By model\n\n")
	p.WriteString("Which model drove the lane, and how it went. The tool columns further down are " +
		"the same for every model, because what a case requires belongs to the scenario. " +
		"What changes here is whether the driving succeeded.\n\n")
	p.WriteString("| Model | Cases run | Reached their bar | Below it | Runs passed | Reliability | " +
		"Calls per case (median) | Slow |\n|---|---:|---:|---:|---:|---:|---:|---:|\n")
	for _, m := range r.Models {
		fmt.Fprintf(p, "| `%s` | %d of %d | %d | %d | %d/%d | %.0f%% | %s | %d |\n",
			m.Model, m.CasesRecorded, r.Totals.Cases, m.CasesHeld, m.CasesBelowBar,
			m.Passed, m.Runs, 100*m.Reliability, m.CallsPerCase, len(m.Slow))
	}
	p.WriteString("\n")
	for _, m := range r.Models {
		if m.CasesRecorded < r.Totals.Cases {
			fmt.Fprintf(p, "> `%s` has no committed run for %d of %d cases.\n\n",
				m.Model, r.Totals.Cases-m.CasesRecorded, r.Totals.Cases)
		}
		if len(m.BelowBar) > 0 {
			fmt.Fprintf(p, "<!-- prose:allow sentence a generated list of case names -->\n> `%s` below its bar on: %s\n\n",
				m.Model, strings.Join(m.BelowBar, ", "))
		}
		if len(m.Slow) > 0 {
			fmt.Fprintf(p, "> `%s` slow on: %s\n\n", m.Model, strings.Join(m.Slow, ", "))
		}
	}
}

// writeCoverageCases is the lane itself, case by case: what each one is for and
// how it went. A manager reading only one table should read this one.
func writeCoverageCases(p *strings.Builder, r mcpToolCoverage) {
	p.WriteString("## The use cases\n\n")
	p.WriteString("One row per case per model that ran it. A case nobody has run appears once, " +
		"marked `not run`, rather than being dropped: a case missing from this table would read " +
		"as a case that does not exist.\n\n")
	p.WriteString("| Case | Model | Result | Passed | Bar | Calls (median) | Turns (median) | Criteria | Requires |\n" +
		"|---|---|---|---:|---:|---:|---:|---|---|\n")
	for _, c := range r.Cases {
		criteria := criteriaNames(c.Name, c.Criteria, r.Criteria)
		link := fmt.Sprintf("[%s](../../e2e/llm/scenarios/%s)", c.Name, c.File)
		if len(c.ByModel) == 0 {
			fmt.Fprintf(p, "| %s | - | not run | - | - | - | - | %s | %s |\n",
				link, criteria, requiresColumn(c))
			continue
		}
		for _, run := range c.ByModel {
			result := "**FAIL**"
			if run.Held {
				result = "pass"
			}
			if run.Slow {
				result += ", slow"
			}
			if run.Regraded {
				result += ", re-graded"
			}
			if len(run.EarlierListing) > 0 {
				result += ", on an earlier listing (" + strings.Join(run.EarlierListing, ", ") + ")"
			}
			fmt.Fprintf(p, "| %s | `%s` | %s | %d/%d | %d | %s | %s | %s | %s |\n",
				link, run.Model, result, run.Passed, run.Runs, run.PassAt,
				run.Efficiency.CallsCell(), run.Efficiency.TurnsCell(), criteria, requiresColumn(c))
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
		fmt.Fprintf(p, "| `%s` | %d | %s | %s |\n", c.Case, c.Number, c.Name, c.Statement)
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
	p.WriteString("A tool this lane has not tried may still be graded. The `Graded by` " +
		"column names the certification tasks that expect\n")
	p.WriteString("the tool as the answer. That lane asks which tool a goal should reach " +
		"for, which this lane cannot express at all:\n")
	p.WriteString("it sees that a name appeared, never whether it was the right first reach. " +
		"`Near miss in` names the tasks that only check\n")
	p.WriteString("a model does not reach for the tool, which says nothing about whether one can use it.\n\n")
	untried := r.Totals.NeverDriven - r.Totals.UndrivenButGraded
	fmt.Fprintf(p, "So of the %d tools no use case requires, **%d are graded elsewhere** as an expected "+
		"answer and %d are untried by any lane. Of the untried, %d appear elsewhere only as a near miss.\n\n",
		r.Totals.NeverDriven, r.Totals.UndrivenButGraded, untried, r.Totals.UndrivenNearMissOnly)
	p.WriteString("A tool in the `Permitted in` column is worse than one with nothing: a case is " +
		"allowed to use it and no case checks that it can.\n\n")
	p.WriteString("| Tool | Tokens | Graded by | Near miss in | Permitted in | Attached to |\n" +
		"|---|---:|---|---|---|---|\n")
	rows := 0
	for _, row := range r.Tools {
		// A tool some case requires as one of a set has its own section above:
		// listing it here too would print it as untried on the page that just
		// said a case reaches for it.
		if row.Driven || row.OneOfASet {
			continue
		}
		rows++
		fmt.Fprintf(p, "| `%s` | %d | %s | %s | %s | %s |\n", row.Name, row.Tokens, joinOrDash(row.GradedBy),
			joinOrDash(row.NearMissIn), joinOrDash(row.MayCall), joinOrDash(row.Agents))
	}
	if rows == 0 {
		p.WriteString("| _every tool has a case_ | - | - | - | - | - |\n")
	}
	p.WriteString("\n")
}

// criteriaNames renders a case's criteria as "8 promises come back as
// suggestions" rather than "8" — a number is only a reference, and the table it
// referenced was not in this repository.
func criteriaNames(caseName string, numbers []int, catalog []criterionRow) string {
	if len(numbers) == 0 {
		return "-"
	}
	named := criterionNames(caseName, catalog)
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

// criterionNames is one case's criteria by number.
func criterionNames(caseName string, catalog []criterionRow) map[int]string {
	named := map[int]string{}
	for _, c := range catalog {
		if c.Case == caseName {
			named[c.Number] = c.Name
		}
	}
	return named
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
		return "-"
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
	p.WriteString("The case fails if no member of the set is called, and passes having called any one of " +
		"them. So a run may never touch the tool named here,\n")
	p.WriteString("and the case's pass rate belongs to the set rather than to this row. That is " +
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
		return "-"
	}
	return strings.Join(backticked(in), ", ")
}

// The two job tables that sit beside the main one: the same scenarios run
// through the neutral harness, and through the REST door. Each answers a
// different question, so neither shares a column with a CLI result.

// summariseDoors folds the MCP models the lane ran, and reads the REST door.
func summariseDoors(t *testing.T, cases []caseRow, ran []string) ([]modelCoverage, doorCoverage) {
	t.Helper()
	models := make([]modelCoverage, 0, len(ran))
	for _, model := range ran {
		models = append(models, summariseModel(cases, model))
	}
	rest, err := readDoor(e2eLLMScenarioDir, e2eLLMRESTDir)
	if err != nil {
		t.Fatalf("reading the REST door's verdicts at %s: %v", e2eLLMRESTDir, err)
	}
	// The page says the REST door is tested on Claude alone; this keeps that true.
	for _, m := range rest.Models {
		if !strings.HasPrefix(m.Model, "claude") {
			t.Fatalf("%s holds a REST verdict, and the REST door is tested on Claude only; remove it", m.Model)
		}
	}
	return models, rest
}

// doorCoverage is the lane as it went through one door other than the MCP
// tools. Cases holds only the jobs some assistant ran through that door.
type doorCoverage struct {
	Models []modelCoverage `json:"models"`
	Cases  []caseRow       `json:"cases"`
}

// readDoor folds one verdict tree the way the MCP tree is folded. Trust is
// counted against every scenario, so a door tried on a handful of jobs says so.
func readDoor(scenarioDir, recordDir string) (doorCoverage, error) {
	cases, err := readE2ELLMCases(scenarioDir, recordDir)
	if err != nil {
		return doorCoverage{}, err
	}
	door := doorCoverage{Models: []modelCoverage{}, Cases: []caseRow{}}
	for _, c := range cases {
		if len(c.ByModel) > 0 {
			door.Cases = append(door.Cases, c)
		}
	}
	for _, model := range modelsThatRan(door.Cases) {
		door.Models = append(door.Models, summariseModel(cases, model))
	}
	return door, nil
}

// onNeutralHarness reports whether a verdict folder holds bridge runs: only a
// CLI route files its verdicts under an @suffix.
func onNeutralHarness(folder string) bool {
	return !strings.Contains(folder, "@")
}

// splitByHarness parts the models into the CLI ones and the neutral-harness
// ones, keeping the order they came in.
func splitByHarness(models []modelCoverage) (cli, neutral []modelCoverage) {
	for _, m := range models {
		if onNeutralHarness(m.Model) {
			neutral = append(neutral, m)
			continue
		}
		cli = append(cli, m)
	}
	return cli, neutral
}

// casesRunBy keeps the jobs at least one of the models ran.
func casesRunBy(cases []caseRow, models []modelCoverage) []caseRow {
	var out []caseRow
	for _, c := range cases {
		for _, m := range models {
			if jobMark(c, m.Model) != "-" {
				out = append(out, c)
				break
			}
		}
	}
	return out
}

// whoAsks names the seat whose passport a job runs on.
func whoAsks(passport string) string {
	switch passport {
	case "rep":
		return "a rep"
	case "readonly":
		return "read-only"
	default:
		return "an admin"
	}
}

func writeJobTable(p *strings.Builder, cases []caseRow, models []modelCoverage, catalog []criterionRow) {
	header := "| Job | What a good answer does | Who asks |"
	rule := "|---|---|---|"
	var headerCells, ruleCells strings.Builder
	for _, m := range models {
		fmt.Fprintf(&headerCells, " `%s`<br>%s |", modelName(m.Model), m.Route)
		ruleCells.WriteString(":---:|")
	}
	p.WriteString(header + headerCells.String() + "\n" + rule + ruleCells.String() + "\n")
	for _, c := range byCaseNumber(cases) {
		fmt.Fprintf(p, "| %s | %s | %s |", c.Title, plainCriteria(c.Name, c.Criteria, catalog), whoAsks(c.Passport))
		for _, m := range models {
			p.WriteString(" " + jobMark(c, m.Model) + " |")
		}
		p.WriteString("\n")
	}
	p.WriteString("\n")
}

func writeCoverageNeutral(p *strings.Builder, r mcpToolCoverage) {
	p.WriteString("## The same jobs on the neutral harness\n\n")
	_, neutral := splitByHarness(r.Models)
	if len(neutral) == 0 {
		p.WriteString("No job has been run through the neutral harness yet.\n\n")
		return
	}
	p.WriteString("These runs give every model the same tools, the same instructions and the same cap " +
		"of turns, so this table is the one that compares one model with another. Its results are " +
		"not mixed into the table above, where each assistant runs with its own vendor's instructions.\n\n")
	writeJobTable(p, casesRunBy(r.Cases, neutral), neutral, r.Criteria)
}

func writeCoverageREST(p *strings.Builder, r mcpToolCoverage) {
	p.WriteString("## The same jobs through the REST API\n\n")
	if len(r.REST.Models) == 0 {
		p.WriteString("No job has been run through the REST API yet.\n\n")
		return
	}
	p.WriteString("An assistant can also reach Margince through the REST API, with the downloadable " +
		"skill and one tool that makes HTTP calls. These runs use the same jobs and the same checks " +
		"as the tables above. The REST door is tested on Claude only, by decision: the other models " +
		"are tested through the MCP tools, and no column here is missing for them.\n\n")
	p.WriteString("| Assistant | How we connected it | Jobs tried | Reliable on |\n|---|---|---:|---:|\n")
	for _, m := range r.REST.Models {
		fmt.Fprintf(p, "| `%s` | %s, REST | %d of %d | %d |\n", m.Name, m.Route, m.CasesRecorded, r.Totals.Cases, m.CasesHeld)
	}
	p.WriteString("\n")
	writeJobTable(p, r.REST.Cases, r.REST.Models, r.Criteria)
}

// calledInHeldRuns names the cases that held where a passing run used tool.
func calledInHeldRuns(cases []caseRow, tool string) []string {
	var named []string
	for _, c := range cases {
		if slices.ContainsFunc(c.ByModel, func(run caseModelRun) bool { return run.Held && slices.Contains(run.ToolsCalled, tool) }) {
			named = append(named, c.Name)
		}
	}
	return named
}

// earlierListings is what changed since each verdict of one tree ran, per `make e2e-llm-report`.
func earlierListings(recordDir string) (map[verdictKey][]string, error) {
	lines, err := mcpe2e.Report(repoRoot)
	if err != nil {
		return nil, err
	}
	out := map[verdictKey][]string{}
	for _, line := range lines {
		if line.Tree == filepath.Base(recordDir) && len(line.Why) > 0 {
			out[verdictKey{model: line.Folder, scenario: line.Scenario}] = line.Why
		}
	}
	return out, nil
}
