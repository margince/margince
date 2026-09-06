// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// docs/reference/mcp-tool-coverage.{json,md} answers, for somebody deciding
// what to put in front of a user: which of the actions this product offers an
// assistant has anybody actually driven end to end, how well it went, and what
// nothing has ever tried.
//
// The lane it reads is the USE-CASE one (e2e/llm) — a real assistant driving a
// whole workflow against a seeded stack, which is the shape a user meets. The
// certification lane grades a different question, one step at a time, and
// [ai-certification.md](ai-certification.md) is its page; mixing the two here
// would put two numbers with different meanings in one column.
//
// A tool no case drives is UNTESTED, not broken. It still costs its tokens on
// every step of every run that carries it, and no lane would notice if a model
// stopped being able to call it.

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

var updateMCPToolCoverage = flag.Bool("update-mcp-tool-coverage", false,
	"rewrite docs/reference/mcp-tool-coverage.{json,md} from the served surface and the use-case lane")

const mcpToolCoverageCommand = "go test ./internal/compose/ -run TestTheMCPToolCoverageIsPublished -update-mcp-tool-coverage"

// The use-case lane's scenarios and its committed verdicts, relative to this
// package. The lane is paid and opt-in; what it WROTE is committed, so this
// page costs nothing to regenerate.
//
// The verdicts sit beside the certification lane's records and are foldered by
// MODEL, because that is what a pass rate belongs to: the lane pins its model
// precisely so a number that moves can be read as the product moving, and a
// verdict filed without the model it came from throws that away.
const (
	e2eLLMScenarioDir  = "../../../e2e/llm/scenarios"
	e2eLLMCriteriaFile = "../../../e2e/llm/criteria.yaml"
	e2eLLMRecordDir    = "aicert/records/mcp_e2e"
	mcpDocsDir         = "../../../docs/reference"
)

type toolCoverageRow struct {
	Name string `json:"name"`
	// Tokens is what this tool alone renders, by the same helper the budget
	// page uses — the two pages must not disagree about one tool's cost.
	Tokens int `json:"tokens"`
	// Agents are the scheduled agents that attach it, so a reader can see what
	// an undriven tool is nevertheless being paid for.
	Agents []string `json:"agents"`
	// MustCall are the cases that REQUIRE this tool: the case fails if the
	// assistant does not call it. MayCall are the cases that merely permit it,
	// which is a licence and not a test — a case can pass having never touched
	// a tool it may call.
	MustCall []string `json:"required_by_cases"`
	MayCall  []string `json:"permitted_in_cases"`
	Driven   bool     `json:"driven"`
	// Measured is what the lane saw across the cases that require this tool.
	// Absent when nothing requires it, or when no run has been committed.
	Measured *toolMeasurement `json:"measured"`
}

// toolMeasurement is how the cases that require one tool actually went.
//
// It is over THOSE cases only. A case is run several times because the lane is
// not deterministic — one bad run in three is the weather and two is a defect —
// so a rate is the only honest summary of it.
type toolMeasurement struct {
	Runs   int `json:"runs"`
	Passed int `json:"passed"`
	// Reliability is Passed/Runs across every case requiring the tool.
	Reliability float64 `json:"reliability"`
	// BelowBar names the cases that did not reach their own pass_at. A case
	// carries its own bar because they are not equally hard, so comparing every
	// case against one number would flatter the easy ones.
	BelowBar []string `json:"cases_below_their_bar"`
}

// criterionRow is one acceptance criterion as a reader meets it: the number a
// scenario declares, and what that number ASKS.
//
// The statement lives in e2e/llm/criteria.yaml rather than here because it is
// the lane's to state, not this page's to invent.
type criterionRow struct {
	Number    int      `json:"number"`
	Name      string   `json:"name"`
	Statement string   `json:"statement"`
	Cases     []string `json:"cases"`
}

type coverageTotals struct {
	Tools              int   `json:"tools"`
	Driven             int   `json:"driven"`
	NeverDriven        int   `json:"never_driven"`
	PermittedNotDriven int   `json:"permitted_but_never_required"`
	UndrivenCost       int   `json:"tokens_served_but_never_driven"`
	Cases              int   `json:"use_case_cases"`
	CasesRecorded      int   `json:"cases_with_a_committed_run"`
	CriteriaCovered    []int `json:"acceptance_criteria_covered"`
}

type caseRow struct {
	Name     string   `json:"name"`
	File     string   `json:"file"`
	Criteria []int    `json:"criteria"`
	Requires []string `json:"requires"`
	Runs     int      `json:"runs"`
	Passed   int      `json:"passed"`
	PassAt   int      `json:"pass_at"`
	Recorded bool     `json:"recorded"`
	Model    string   `json:"model"`
}

type mcpToolCoverage struct {
	Note     string            `json:"note"`
	Totals   coverageTotals    `json:"totals"`
	Cases    []caseRow         `json:"cases"`
	Criteria []criterionRow    `json:"criteria"`
	Tools    []toolCoverageRow `json:"tools"`
}

const mcpToolCoverageNote = "Generated by `" + mcpToolCoverageCommand + "`; do not edit by hand. " +
	"It reads the use-case lane only — the scenarios under e2e/llm/scenarios and the verdicts the " +
	"lane commits beside them. A tool counts as DRIVEN when at least one case REQUIRES it " +
	"(must_call); a case that merely permits a tool (may_call) can pass without ever calling it, " +
	"so permission is not coverage. Token counts are the budget page's own, one tool rendered alone."

func TestTheMCPToolCoverageIsPublished(t *testing.T) {
	specs := servedSurface(t).Specs()

	cases, err := readE2ELLMCases(e2eLLMScenarioDir, e2eLLMRecordDir)
	if err != nil {
		t.Fatalf("reading the use-case lane at %s: %v", e2eLLMScenarioDir, err)
	}
	// Under-recognition is the one way this page must not break: a reader that
	// found no case would publish "nothing is driven" and look like a finding
	// rather than a broken scan.
	if len(cases) == 0 {
		t.Fatalf("read no use-case scenario from %s — a scan that finds nothing publishes a page "+
			"saying the surface is undriven", e2eLLMScenarioDir)
	}

	catalog, err := readCriteria(e2eLLMCriteriaFile)
	if err != nil {
		t.Fatalf("reading the criteria catalog at %s: %v", e2eLLMCriteriaFile, err)
	}

	attached := map[string][]string{}
	for _, agent := range mustScheduledAgents() {
		for _, tool := range agent.Tools {
			attached[tool] = append(attached[tool], agent.Name)
		}
	}

	report := mcpToolCoverage{Note: mcpToolCoverageNote, Cases: cases}
	report.Totals.Cases = len(cases)
	criteria := map[int]bool{}
	for _, c := range cases {
		if c.Recorded {
			report.Totals.CasesRecorded++
		}
		for _, n := range c.Criteria {
			criteria[n] = true
		}
	}
	for n := range criteria {
		report.Totals.CriteriaCovered = append(report.Totals.CriteriaCovered, n)
	}
	sort.Ints(report.Totals.CriteriaCovered)
	for _, n := range report.Totals.CriteriaCovered {
		row, named := catalog[n]
		if !named {
			t.Errorf("scenarios declare criterion %d and %s does not name it — a grade against a "+
				"number nothing in this repository explains cannot be read by the person it is for",
				n, e2eLLMCriteriaFile)
			continue
		}
		for _, c := range cases {
			for _, declared := range c.Criteria {
				if declared == n {
					row.Cases = append(row.Cases, c.Name)
				}
			}
		}
		row.Cases = sortedCopy(row.Cases)
		report.Criteria = append(report.Criteria, row)
	}

	for _, spec := range specs {
		row := toolCoverageRow{
			Name:   spec.Name,
			Tokens: oneToolTokens(spec),
			Agents: sortedCopy(attached[spec.Name]),
		}
		for _, c := range cases {
			if listHas(c.Requires, spec.Name) {
				row.MustCall = append(row.MustCall, c.Name)
			}
		}
		row.MayCall = permittedCases(cases, spec.Name)
		row.MustCall = sortedCopy(row.MustCall)
		row.Driven = len(row.MustCall) > 0
		row.Measured = measureCases(cases, row.MustCall)
		report.Tools = append(report.Tools, row)
	}
	sort.Slice(report.Tools, func(i, j int) bool {
		if report.Tools[i].Driven != report.Tools[j].Driven {
			return !report.Tools[i].Driven
		}
		if report.Tools[i].Tokens != report.Tools[j].Tokens {
			return report.Tools[i].Tokens > report.Tools[j].Tokens
		}
		return report.Tools[i].Name < report.Tools[j].Name
	})
	for _, row := range report.Tools {
		report.Totals.Tools++
		if row.Driven {
			report.Totals.Driven++
			continue
		}
		report.Totals.NeverDriven++
		report.Totals.UndrivenCost += row.Tokens
		if len(row.MayCall) > 0 {
			report.Totals.PermittedNotDriven++
		}
	}

	payload, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatalf("encoding the coverage report: %v", err)
	}
	syncMCPToolCoverage(t, filepath.Join(mcpDocsDir, "mcp-tool-coverage.json"), append(payload, '\n'))
	syncMCPToolCoverage(t, filepath.Join(mcpDocsDir, "mcp-tool-coverage.md"), renderMCPToolCoveragePage(report))
}

// The scenario file's shape, and the verdict the lane writes beside it.
var (
	e2eScenarioName  = regexp.MustCompile(`(?m)^name:\s*(\S+)\s*$`)
	e2eCriteria      = regexp.MustCompile(`(?m)^criteria:\s*\[([^\]]*)\]`)
	e2eMustCallBlock = regexp.MustCompile(`(?ms)^must_call:\n((?:\s*-\s*\S+\n)+)`)
	e2eMayCallBlock  = regexp.MustCompile(`(?ms)^may_call:\n((?:\s*-\s*\S+\n)+)`)
	e2eToolItem      = regexp.MustCompile(`(?m)^\s*-\s*(\S+)\s*$`)
	e2eCriterion     = regexp.MustCompile(`\d+`)
)

// verdictKey is what a committed verdict is filed under. The model is half of
// it: a pass rate belongs to the model that produced it, and two models that
// ran the same case have two answers, not one.
type verdictKey struct {
	model    string
	scenario string
}

type e2eVerdict struct {
	Scenario string `json:"scenario"`
	Passed   int    `json:"passed"`
	Runs     int    `json:"runs"`
	PassAt   int    `json:"pass_at"`
	// Model is the folder the verdict was filed under, not a field in it: the
	// lane names the directory for the model it pinned.
	Model string `json:"-"`
}

// readE2ELLMCases reads every use-case scenario and joins it to the verdict the
// lane committed for it, if one has been.
//
// A scenario with no verdict is reported as a case with no run rather than
// dropped: "nobody has run this" and "this failed" are different answers and a
// page that could not tell them apart would be worse than no page.
func readE2ELLMCases(scenarioDir, recordDir string) ([]caseRow, error) {
	verdicts, err := readE2ELLMVerdicts(recordDir)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(scenarioDir)
	if err != nil {
		return nil, err
	}
	var cases []caseRow
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}
		body, readErr := os.ReadFile(filepath.Join(scenarioDir, entry.Name()))
		if readErr != nil {
			return nil, readErr
		}
		text := string(body)
		row := caseRow{Name: strings.TrimSuffix(entry.Name(), ".yaml"), File: entry.Name()}
		if got := e2eScenarioName.FindStringSubmatch(text); len(got) == 2 {
			row.Name = got[1]
		}
		if got := e2eCriteria.FindStringSubmatch(text); len(got) == 2 {
			for _, n := range e2eCriterion.FindAllString(got[1], -1) {
				row.Criteria = append(row.Criteria, atoiOrZero(n))
			}
		}
		row.Requires = sortedCopy(toolsInBlock(e2eMustCallBlock, text))
		if v, ok := verdictFor(verdicts, row.Name); ok {
			row.Recorded, row.Runs, row.Passed, row.PassAt = true, v.Runs, v.Passed, v.PassAt
			row.Model = v.Model
		}
		cases = append(cases, row)
	}
	sort.Slice(cases, func(i, j int) bool { return cases[i].File < cases[j].File })
	return cases, nil
}

// readE2ELLMVerdicts reads the committed verdicts. A missing directory is not
// an error: it means the paid lane has not been run on this checkout, which the
// page says outright.
func readE2ELLMVerdicts(dir string) (map[verdictKey]e2eVerdict, error) {
	out := map[verdictKey]e2eVerdict{}
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	for _, model := range entries {
		if !model.IsDir() {
			continue
		}
		files, readErr := os.ReadDir(filepath.Join(dir, model.Name()))
		if readErr != nil {
			return nil, readErr
		}
		for _, entry := range files {
			if !strings.HasSuffix(entry.Name(), ".json") {
				continue
			}
			body, fileErr := os.ReadFile(filepath.Join(dir, model.Name(), entry.Name()))
			if fileErr != nil {
				return nil, fileErr
			}
			var v e2eVerdict
			if decodeErr := json.Unmarshal(body, &v); decodeErr != nil {
				return nil, fmt.Errorf("%s/%s: %w", model.Name(), entry.Name(), decodeErr)
			}
			v.Model = model.Name()
			// Keyed by MODEL AND SCENARIO. Keying on the scenario alone let the
			// last model directory read win, so a page could show one model's
			// pass rate beside another's on the next row and say nothing about
			// it. Today one model has run; the bug would appear silently on the
			// day a second one does, which is the worst time to find it.
			out[verdictKey{model: model.Name(), scenario: v.Scenario}] = v
		}
	}
	return out, nil
}

// readCriteria returns the catalog keyed by number.
//
// Parsed with the YAML decoder this module already depends on, not by hand: a
// bespoke reader for a folded block is how the first version of this silently
// published the first line of every statement and dropped the rest.
func readCriteria(path string) (map[int]criterionRow, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var file struct {
		Criteria map[int]struct {
			Name      string `yaml:"name"`
			Statement string `yaml:"statement"`
		} `yaml:"criteria"`
	}
	if err := yaml.Unmarshal(body, &file); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	out := make(map[int]criterionRow, len(file.Criteria))
	for number, entry := range file.Criteria {
		out[number] = criterionRow{
			Number:    number,
			Name:      entry.Name,
			Statement: strings.TrimSpace(entry.Statement),
			Cases:     []string{},
		}
	}
	return out, nil
}

// verdictFor picks one model's verdict for a case, by model name in order, so
// the page is byte-stable and every row on it comes from the same model rather
// than from whichever directory the filesystem listed first.
//
// It reports the FIRST model alphabetically that ran the case. That is a choice
// the page states in its own totals, and it is only visible once a second model
// has run — at which point this wants replacing with a column per model rather
// than a rule for picking one.
func verdictFor(verdicts map[verdictKey]e2eVerdict, scenario string) (e2eVerdict, bool) {
	var chosen e2eVerdict
	found := false
	for key, v := range verdicts {
		if key.scenario != scenario {
			continue
		}
		if !found || key.model < chosen.Model {
			chosen, found = v, true
		}
	}
	return chosen, found
}

func toolsInBlock(block *regexp.Regexp, text string) []string {
	got := block.FindStringSubmatch(text)
	if len(got) != 2 {
		return nil
	}
	var out []string
	for _, item := range e2eToolItem.FindAllStringSubmatch(got[1], -1) {
		out = append(out, item[1])
	}
	return out
}

// permittedCases names the cases that allow a tool without requiring it.
func permittedCases(cases []caseRow, tool string) []string {
	var out []string
	for _, c := range cases {
		if listHas(c.Requires, tool) {
			continue
		}
		body, err := os.ReadFile(filepath.Join(e2eLLMScenarioDir, c.File))
		if err != nil {
			continue
		}
		if listHas(toolsInBlock(e2eMayCallBlock, string(body)), tool) {
			out = append(out, c.Name)
		}
	}
	return sortedCopy(out)
}

// measureCases folds the committed verdicts of the cases requiring one tool.
func measureCases(cases []caseRow, names []string) *toolMeasurement {
	if len(names) == 0 {
		return nil
	}
	m := toolMeasurement{BelowBar: []string{}}
	for _, c := range cases {
		if !listHas(names, c.Name) || !c.Recorded {
			continue
		}
		m.Runs += c.Runs
		m.Passed += c.Passed
		if c.Passed < c.PassAt {
			m.BelowBar = append(m.BelowBar, c.Name)
		}
	}
	if m.Runs == 0 {
		return nil
	}
	m.Reliability = float64(m.Passed) / float64(m.Runs)
	sort.Strings(m.BelowBar)
	return &m
}

// listHas reports whether a name is in a list. Named for the question rather
// than the container, and not `contains`: this package already has one of those
// with a different argument order, and two spellings of the same predicate is
// how a call site comes to read backwards.
func listHas(in []string, want string) bool {
	for _, v := range in {
		if v == want {
			return true
		}
	}
	return false
}

func atoiOrZero(s string) int {
	n := 0
	for _, r := range s {
		n = n*10 + int(r-'0')
	}
	return n
}

// sortedCopy returns the names sorted and de-duplicated, so a rendered row is
// byte-stable whatever order the readers found them in.
func sortedCopy(in []string) []string {
	if len(in) == 0 {
		return []string{}
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, v := range in {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	sort.Strings(out)
	return out
}

func syncMCPToolCoverage(t *testing.T, path string, want []byte) {
	t.Helper()
	if *updateMCPToolCoverage {
		if err := os.WriteFile(path, want, 0o644); err != nil {
			t.Fatalf("rewriting %s: %v", path, err)
		}
		return
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the committed artifact %s: %v\nRegenerate with: %s", path, err, mcpToolCoverageCommand)
	}
	if bytes.Equal(got, want) {
		return
	}
	t.Errorf("%s is stale — it no longer matches the served surface and the use-case lane.\n"+
		"Regenerate with: %s\nand commit the result with the change that moved it.\n%s",
		path, mcpToolCoverageCommand, firstMCPInfoDifference(string(got), string(want)))
}

func renderMCPToolCoveragePage(r mcpToolCoverage) []byte {
	var p strings.Builder
	p.WriteString("# What the assistant can be relied on to do\n\n")
	p.WriteString("<!-- Generated together with mcp-tool-coverage.json; do not edit by hand. -->\n\n")
	p.WriteString(r.Note + "\n\n")
	p.WriteString("**This page is generated, and an edit made here is lost.**\n\n")
	writeCoverageHowToRead(&p)
	writeCoverageTotals(&p, r)
	writeCoverageCases(&p, r)
	writeCoverageCriteria(&p, r)
	writeCoverageReliable(&p, r)
	writeCoverageFailing(&p, r)
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
	fmt.Fprintf(p, "| … **no case requires** | %d |\n", r.Totals.NeverDriven)
	fmt.Fprintf(p, "| … of those, permitted somewhere but never required | %d |\n", r.Totals.PermittedNotDriven)
	fmt.Fprintf(p, "| Prompt tokens spent on tools no case requires | %d |\n", r.Totals.UndrivenCost)
	fmt.Fprintf(p, "| Use cases | %d |\n", r.Totals.Cases)
	fmt.Fprintf(p, "| … with a committed run | %d |\n", r.Totals.CasesRecorded)
	fmt.Fprintf(p, "| Acceptance criteria covered | %s |\n\n", criteriaList(r.Totals.CriteriaCovered))
	if r.Totals.CasesRecorded < r.Totals.Cases {
		fmt.Fprintf(p, "> **%d of %d cases have no committed run.** Their rows below say `not run` "+
			"rather than a rate — nobody has paid for the answer yet.\n\n",
			r.Totals.Cases-r.Totals.CasesRecorded, r.Totals.Cases)
	}
}

// writeCoverageCases is the lane itself, case by case: what each one is for and
// how it went. A manager reading only one table should read this one.
func writeCoverageCases(p *strings.Builder, r mcpToolCoverage) {
	p.WriteString("## The use cases\n\n")
	p.WriteString("| Case | Result | Passed | Bar | Model | Criteria | Requires |\n" +
		"|---|---|---:|---:|---|---|---|\n")
	for _, c := range r.Cases {
		result, passed, bar, model := "not run", "—", "—", "—"
		if c.Recorded {
			result = "**FAIL**"
			if c.Passed >= c.PassAt {
				result = "pass"
			}
			passed = fmt.Sprintf("%d/%d", c.Passed, c.Runs)
			bar = fmt.Sprintf("%d", c.PassAt)
			model = "`" + c.Model + "`"
		}
		fmt.Fprintf(p, "| [%s](../../e2e/llm/scenarios/%s) | %s | %s | %s | %s | %s | %s |\n",
			c.Name, c.File, result, passed, bar, model,
			criteriaNames(c.Criteria, r.Criteria), joinOrDash(c.Requires))
	}
	p.WriteString("\n")
}

// writeCoverageCriteria states what each number a case declares actually asks.
// Without it the case table grades against numbers a reader cannot look up.
func writeCoverageCriteria(p *strings.Builder, r mcpToolCoverage) {
	p.WriteString("## What the criteria ask\n\n")
	p.WriteString("The numbers in the table above, in words. Source: " +
		"[`e2e/llm/criteria.yaml`](../../e2e/llm/criteria.yaml).\n\n")
	p.WriteString("| # | Criterion | What it asks | Cases |\n|---:|---|---|---|\n")
	for _, c := range r.Criteria {
		fmt.Fprintf(p, "| %d | **%s** | %s | %s |\n",
			c.Number, c.Name, c.Statement, joinOrDash(c.Cases))
	}
	p.WriteString("\n")
}

// writeCoverageReliable is the first question: what can I put in front of
// somebody. Only a tool whose every covering case cleared its own bar with a
// clean rate qualifies.
func writeCoverageReliable(p *strings.Builder, r mcpToolCoverage) {
	p.WriteString("## 1. What you can rely on\n\n")
	p.WriteString("Every run of every case requiring this tool passed.\n\n")
	p.WriteString("| Tool | Reliability | Runs | Required by |\n|---|---:|---:|---|\n")
	rows := 0
	for _, row := range r.Tools {
		m := row.Measured
		if m == nil || m.Reliability < 1 || len(m.BelowBar) > 0 {
			continue
		}
		rows++
		fmt.Fprintf(p, "| `%s` | %.2f | %d | %s |\n", row.Name, m.Reliability, m.Runs, joinOrDash(row.MustCall))
	}
	if rows == 0 {
		p.WriteString("| _nothing yet_ | - | - | - |\n")
	}
	p.WriteString("\n")
}

// writeCoverageFailing is the second question: what is wrong today. A tool
// appears here when it was driven and a run did not pass — including a case
// that cleared its bar while losing a run, which is a rate worth seeing.
func writeCoverageFailing(p *strings.Builder, r mcpToolCoverage) {
	p.WriteString("## 2. What is failing now\n\n")
	p.WriteString("Driven, and not every run passed. Open the case to see what was asked.\n\n")
	p.WriteString("| Tool | Reliability | Passed | Below its bar | Required by |\n|---|---:|---:|---|---|\n")
	rows := 0
	for _, row := range r.Tools {
		m := row.Measured
		if m == nil || (m.Reliability >= 1 && len(m.BelowBar) == 0) {
			continue
		}
		rows++
		fmt.Fprintf(p, "| `%s` | %.2f | %d/%d | %s | %s |\n", row.Name, m.Reliability,
			m.Passed, m.Runs, joinOrDash(m.BelowBar), joinOrDash(row.MustCall))
	}
	if rows == 0 {
		p.WriteString("| _nothing driven is failing_ | - | - | - | - |\n")
	}
	p.WriteString("\n")
}

// writeCoverageUndriven is the third question, and the one this page exists
// for: what has nobody written a case for. Ordered by what each costs, because
// that is the bill being paid for the untried thing.
func writeCoverageUndriven(p *strings.Builder, r mcpToolCoverage) {
	p.WriteString("## 3. What no case requires\n\n")
	p.WriteString("**Untried, not broken.** Each is offered to the assistant on every step of every " +
		"run that carries it, and no case would notice if a model\n")
	p.WriteString("stopped being able to call it. A tool in the `permitted` column is worse than one " +
		"with nothing: a case is allowed to use it and no case checks that it can.\n\n")
	p.WriteString("| Tool | Tokens | Permitted in | Attached to |\n|---|---:|---|---|\n")
	rows := 0
	for _, row := range r.Tools {
		if row.Driven {
			continue
		}
		rows++
		fmt.Fprintf(p, "| `%s` | %d | %s | %s |\n",
			row.Name, row.Tokens, joinOrDash(row.MayCall), joinOrDash(row.Agents))
	}
	if rows == 0 {
		p.WriteString("| _every tool has a case_ | - | - | - |\n")
	}
	p.WriteString("\n")
}

// criteriaNames renders a case's criteria as "8 promises come back as
// suggestions" rather than "8" — a number is only a reference, and the table it
// referenced was not in this repository.
func criteriaNames(numbers []int, catalog []criterionRow) string {
	if len(numbers) == 0 {
		return "—"
	}
	named := map[int]string{}
	for _, c := range catalog {
		named[c.Number] = c.Name
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

// criteriaList renders acceptance-criteria numbers, or a dash when a case
// declares none — an empty cell reads as a rendering bug rather than as "none".
func criteriaList(in []int) string {
	if len(in) == 0 {
		return "—"
	}
	out := make([]string, 0, len(in))
	for _, n := range in {
		out = append(out, fmt.Sprintf("%d", n))
	}
	return strings.Join(out, ", ")
}

// joinOrDash renders a list as backticked names, or a dash when it is empty.
func joinOrDash(in []string) string {
	if len(in) == 0 {
		return "—"
	}
	quoted := make([]string, 0, len(in))
	for _, v := range in {
		quoted = append(quoted, "`"+v+"`")
	}
	return strings.Join(quoted, ", ")
}
