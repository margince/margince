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
	// GradedBy are the certification tasks whose corpus names this tool. That
	// lane grades tool SELECTION — which tool a goal should reach for, and
	// which plausible neighbour it must avoid — and the use-case lane cannot
	// express it: check.py sees that a name appeared, never whether it was the
	// right first reach. So a tool no case REQUIRES may still be graded, and a
	// page that reported the two as one silence would name work undone that is
	// done, on a different question.
	GradedBy []string `json:"graded_by_certification_tasks"`
	// MeasuredByModel is what the lane saw across the cases that require this
	// tool, PER MODEL. Empty when nothing requires it, or when no run has been
	// committed. Keyed by model because reliability is a property of the pair:
	// one model driving a tool well says nothing about another driving it.
	MeasuredByModel map[string]*toolMeasurement `json:"measured_by_model"`
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
	// Case is half of a criterion's identity. The numbers are PER CASE: case 1
	// criterion 1 and case 2 criterion 1 are different criteria that share a
	// digit, and the suites that pin them say "case 4 criterion 5" rather than a
	// bare number for exactly that reason.
	Case      string `json:"case"`
	Number    int    `json:"number"`
	Name      string `json:"name"`
	Statement string `json:"statement"`
}

type coverageTotals struct {
	Tools              int `json:"tools"`
	Driven             int `json:"driven"`
	NeverDriven        int `json:"never_driven"`
	PermittedNotDriven int `json:"permitted_but_never_required"`
	UndrivenCost       int `json:"tokens_served_but_never_driven"`
	Cases              int `json:"use_case_cases"`
	CriteriaNamed      int `json:"acceptance_criteria_named"`
	// UnitTools is what the shipped units add to the SAME registry the core
	// catalog is served from. They are not in Tools above: this package cannot
	// import a unit (each is its own module and the DAG forbids the edge), so
	// they are read from what the composer published and cannot be priced here.
	UnitTools int `json:"tools_added_by_shipped_units"`
	// UndrivenButGraded is how many of the never-driven tools the certification
	// corpus nevertheless names. It is the difference between "untried" and
	// "untried by this lane".
	UndrivenButGraded int `json:"never_driven_but_graded_elsewhere"`
}

type caseRow struct {
	Name     string   `json:"name"`
	File     string   `json:"file"`
	Criteria []int    `json:"criteria"`
	Requires []string `json:"requires"`
	// ByModel is one entry per model the lane has been run with, because a pass
	// rate belongs to the model that produced it. It was a scalar while one
	// model had run, and collapsing several into one would publish whichever
	// directory sorted first as though it were the answer — a number describing
	// something other than what it names, which is the defect this page exists
	// to have stopped doing.
	ByModel []caseModelRun `json:"by_model"`
}

// caseModelRun is one model's result on one case.
type caseModelRun struct {
	Model  string `json:"model"`
	Runs   int    `json:"runs"`
	Passed int    `json:"passed"`
	PassAt int    `json:"pass_at"`
	// Held is whether the case reached its own bar for this model. Stated
	// rather than left to a reader to compute, because pass_at differs per case
	// and comparing passed/runs across cases without it flatters the easy ones.
	Held bool `json:"held"`
}

// modelCoverage is the whole lane as ONE model ran it. Driven/never-driven is
// not in here on purpose: what a case REQUIRES is a property of the scenario, so
// the surface a lane exercises is the same whoever drives it. What changes per
// model is whether the driving SUCCEEDED, and only that is reported per model.
type modelCoverage struct {
	Model         string `json:"model"`
	CasesRecorded int    `json:"cases_with_a_committed_run"`
	CasesHeld     int    `json:"cases_that_reached_their_bar"`
	CasesBelowBar int    `json:"cases_below_their_bar"`
	Runs          int    `json:"runs"`
	Passed        int    `json:"passed"`
	// Reliability is Passed/Runs over every committed run. It answers "how often
	// did this model do the job", which a count of held cases cannot: a case
	// scraping its bar two runs in three and one passing all three are both held.
	Reliability float64 `json:"reliability"`
	// BelowBar names them, because a rate with no names is a number nobody can
	// act on.
	BelowBar []string `json:"cases_below_their_bar_named"`
}

type mcpToolCoverage struct {
	Note   string         `json:"note"`
	Totals coverageTotals `json:"totals"`
	// Models is the lane per model that has run it. Empty until a paid sweep is
	// committed, which the page says outright rather than implying nothing works.
	Models []modelCoverage `json:"models"`
	// Judges is how well each candidate JUDGE reads the lane's criteria, scored
	// against human-authored fixtures. It rides on this page rather than its own
	// because a pass rate and the accuracy of whoever decided it are one fact: a
	// reader trusting the first without the second is trusting a number whose
	// error bar nobody showed them.
	Judges []judgeEvalRow `json:"judges"`
	// Excluded is what no judge is charged for, carried onto the page so the
	// exemption is visible beside the scores it changes rather than only in the
	// source that applies it.
	Excluded map[string]string `json:"judge_trial_excluded"`
	Cases    []caseRow         `json:"cases"`
	Criteria []criterionRow    `json:"criteria"`
	Tools    []toolCoverageRow `json:"tools"`
	// UnitTools are the agent tools the shipped units contribute, read from
	// each unit's published manifest.
	UnitTools []unitTool `json:"unit_tools"`
	// Agents is Surface B: the scheduled agents and the allowlist each one is
	// narrowed to. A scheduled run never sees the whole catalog, so reporting
	// one coverage number over both surfaces reports a menu nobody is served.
	Agents []agentSurfaceRow `json:"scheduled_agent_surface"`
}

// agentSurfaceRow is one scheduled agent's declared allowlist — Surface B as
// the contract states it, not as a run happened to use it.
type agentSurfaceRow struct {
	Name  string   `json:"name"`
	Tools []string `json:"tools"`
}

const mcpToolCoverageNote = "Generated by `" + mcpToolCoverageCommand + "`; do not edit by hand. " +
	"It reads the use-case lane only — the scenarios under e2e/llm/scenarios and the verdicts the " +
	"lane commits beside them. A tool counts as DRIVEN when at least one case REQUIRES it " +
	"(must_call); a case that merely permits a tool (may_call) can pass without ever calling it, " +
	"so permission is not coverage. Token counts are the budget page's own, one tool rendered alone."

// A scenario that DECLARES required tools and reads as requiring none is the
// shape this page cannot see from its own output: the lane enforces the tools,
// the page reports the case as driving nothing, and every tool it names drifts
// into the untried column. It is the same failure the surface census exists for,
// one level down — a reader that fails short and reports a smaller tree.
//
// Held here rather than trusted to review, because the two readers of this
// schema are in different languages: e2e/llm/check.py is the lane's and this
// file's regexes are the page's, and nothing makes them agree by construction.
func TestEveryDeclaredMustCallIsRecognisedByThePage(t *testing.T) {
	entries, err := os.ReadDir(e2eLLMScenarioDir)
	if err != nil {
		t.Fatalf("reading the scenarios at %s: %v", e2eLLMScenarioDir, err)
	}
	checked := 0
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}
		body, readErr := os.ReadFile(filepath.Join(e2eLLMScenarioDir, entry.Name()))
		if readErr != nil {
			t.Fatalf("reading %s: %v", entry.Name(), readErr)
		}
		text := string(body)
		if !e2eMustCallDeclared.MatchString(text) {
			continue
		}
		checked++
		if len(toolsInBlock(e2eMustCallBlock, e2eMustCallInline, text)) == 0 {
			t.Errorf("%s declares must_call and this page reads no tool from it: the lane will "+
				"enforce those tools and the page will report the case as requiring none, which "+
				"moves every one of them into the untried column without anything failing",
				entry.Name())
		}
	}
	// The scan itself must not fail short: a glob that matched nothing would
	// report every scenario sound.
	if checked == 0 {
		t.Fatalf("no scenario under %s declares must_call — a scan finding nothing here would "+
			"pass while reading an empty tree", e2eLLMScenarioDir)
	}
}

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
	var agentSurface []agentSurfaceRow
	for _, agent := range mustScheduledAgents() {
		for _, tool := range agent.Tools {
			attached[tool] = append(attached[tool], agent.Name)
		}
		agentSurface = append(agentSurface, agentSurfaceRow{Name: agent.Name, Tools: sortedCopy(agent.Tools)})
	}
	sort.Slice(agentSurface, func(i, j int) bool { return agentSurface[i].Name < agentSurface[j].Name })

	units, err := extensionTools(extensionsDir)
	if err != nil {
		t.Fatalf("reading the unit manifests under %s: %v", extensionsDir, err)
	}

	names := make([]string, 0, len(specs))
	for _, spec := range specs {
		names = append(names, spec.Name)
	}
	graded, err := corpusGradedTools(corpusDir, names)
	if err != nil {
		t.Fatalf("reading the certification corpus at %s: %v", corpusDir, err)
	}

	report := mcpToolCoverage{Note: mcpToolCoverageNote, Cases: cases, UnitTools: units, Agents: agentSurface}
	report.Totals.UnitTools = len(units)
	report.Totals.Cases = len(cases)
	judges, err := readJudgeEvals(t, judgeEvalRecordDir)
	if err != nil {
		t.Fatalf("reading the judge eval records at %s: %v", judgeEvalRecordDir, err)
	}
	report.Judges = judges
	report.Excluded = judgeEvalHarnessArtifacts.Reasons()
	judgeEvalHarnessArtifacts.AssertAllMatched(t)

	ran := modelsThatRan(cases)
	for _, model := range ran {
		report.Models = append(report.Models, summariseModel(cases, model))
	}
	for _, c := range cases {
		named, ok := catalog[c.Name]
		if !ok {
			t.Errorf("case %s declares criteria and %s names none of them — a grade against a "+
				"number nothing in this repository explains cannot be read by the person it is for",
				c.Name, e2eLLMCriteriaFile)
			continue
		}
		for _, n := range c.Criteria {
			row, isNamed := named[n]
			if !isNamed {
				t.Errorf("case %s declares criterion %d and %s does not name it", c.Name, n, e2eLLMCriteriaFile)
				continue
			}
			report.Criteria = append(report.Criteria, row)
			// A placeholder is worse than a missing entry: it satisfies the
			// lookup above, so the page publishes a grade against a criterion
			// whose statement says the criterion is not written down.
			if strings.Contains(row.Name, "NEEDS A STATEMENT") {
				t.Errorf("case %s criterion %d is a placeholder in %s — write the one sentence "+
					"saying what it asks, derived from the scenario that tests it",
					c.Name, n, e2eLLMCriteriaFile)
				continue
			}
			report.Totals.CriteriaNamed++
		}
	}
	sort.Slice(report.Criteria, func(i, j int) bool {
		if report.Criteria[i].Case != report.Criteria[j].Case {
			return report.Criteria[i].Case < report.Criteria[j].Case
		}
		return report.Criteria[i].Number < report.Criteria[j].Number
	})

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
		row.GradedBy = graded[spec.Name]
		row.MeasuredByModel = map[string]*toolMeasurement{}
		for _, model := range ran {
			if m := measureCases(cases, row.MustCall, model); m != nil {
				row.MeasuredByModel[model] = m
			}
		}
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
		if len(row.GradedBy) > 0 {
			report.Totals.UndrivenButGraded++
		}
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
	e2eScenarioName = regexp.MustCompile(`(?m)^name:\s*(\S+)\s*$`)
	e2eCriteria     = regexp.MustCompile(`(?m)^criteria:\s*\[([^\]]*)\]`)
	// A LIST ITEM IS NOT THE ONLY LINE A BLOCK HOLDS. These scenarios carry their
	// reasoning inline — a comment above the tool it explains — and a pattern that
	// admits only item lines stops at the first one. That is not a partial read:
	// a block whose FIRST line is a comment matched nothing at all, so eight cases
	// reported an empty may_call while permitting three to five tools each.
	//
	// Under-recognition again, in a reader rather than a gate, and silent in the
	// same way: the page published a smaller "permitted" set and nothing failed.
	e2eMustCallBlock = regexp.MustCompile(`(?ms)^must_call:\n((?:[ \t]*(?:#[^\n]*|-[ \t]*\S+)?\n)+)`)
	e2eMayCallBlock  = regexp.MustCompile(`(?ms)^may_call:\n((?:[ \t]*(?:#[^\n]*|-[ \t]*\S+)?\n)+)`)
	// The inline form of the same key. e2e/llm/check.py accepts both shapes, and
	// a reader here that knew only the block form answered "requires nothing" for
	// a case whose tools the lane was enforcing — a census failing short in the
	// page whose subject is a census.
	e2eMustCallInline = regexp.MustCompile(`(?m)^must_call:[ \t]*\[([^\]]*)\]`)
	e2eMayCallInline  = regexp.MustCompile(`(?m)^may_call:[ \t]*\[([^\]]*)\]`)
	e2eInlineItem     = regexp.MustCompile(`[^,\s\[\]]+`)
	// Whether a scenario DECLARES the key at all, in either shape. The gate below
	// compares this against what was parsed: a key that is present and yields no
	// tool is the shape that cannot be seen in the output.
	e2eMustCallDeclared = regexp.MustCompile(`(?m)^must_call:`)
	e2eToolItem         = regexp.MustCompile(`(?m)^\s*-\s*(\S+)\s*$`)
	e2eCriterion        = regexp.MustCompile(`\d+`)
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
		row.Requires = sortedCopy(toolsInBlock(e2eMustCallBlock, e2eMustCallInline, text))
		row.ByModel = runsFor(verdicts, row.Name)
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
func readCriteria(path string) (map[string]map[int]criterionRow, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var file struct {
		Cases map[string]map[int]struct {
			Name      string `yaml:"name"`
			Statement string `yaml:"statement"`
		} `yaml:"cases"`
	}
	if err := yaml.Unmarshal(body, &file); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	out := make(map[string]map[int]criterionRow, len(file.Cases))
	for name, numbered := range file.Cases {
		out[name] = make(map[int]criterionRow, len(numbered))
		for number, entry := range numbered {
			out[name][number] = criterionRow{
				Case:      name,
				Number:    number,
				Name:      entry.Name,
				Statement: strings.TrimSpace(entry.Statement),
			}
		}
	}
	return out, nil
}

// runsFor collects each model's result on one scenario, ordered by model name.
//
// It replaced a chooser that returned ONE verdict per scenario, picking the
// lowest model name. That was correct while one model had run and silently
// wrong the moment a second did: the page would have shown one model's pass
// rate under a heading that named the case, not the model.
func runsFor(verdicts map[verdictKey]e2eVerdict, scenario string) []caseModelRun {
	var out []caseModelRun
	for key, v := range verdicts {
		if key.scenario != scenario {
			continue
		}
		out = append(out, caseModelRun{
			Model:  v.Model,
			Runs:   v.Runs,
			Passed: v.Passed,
			PassAt: v.PassAt,
			Held:   v.PassAt > 0 && v.Passed >= v.PassAt,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Model < out[j].Model })
	return out
}

// toolsInBlock reads one tool key in EITHER shape the lane accepts — the block
// list and the inline `[a, b]` — because check.py accepts both and a page that
// knew one of them would report a case as requiring nothing while the lane
// enforced its tools.
func toolsInBlock(block, inline *regexp.Regexp, text string) []string {
	if got := inline.FindStringSubmatch(text); len(got) == 2 {
		return e2eInlineItem.FindAllString(got[1], -1)
	}
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
		if listHas(toolsInBlock(e2eMayCallBlock, e2eMayCallInline, string(body)), tool) {
			out = append(out, c.Name)
		}
	}
	return sortedCopy(out)
}

// measureCases folds the committed verdicts of the cases requiring one tool,
// for ONE model. A fold across models would average a strong driver with a weak
// one and call the result the tool's reliability, which is nobody's experience
// of it.
func measureCases(cases []caseRow, names []string, model string) *toolMeasurement {
	if len(names) == 0 {
		return nil
	}
	m := toolMeasurement{BelowBar: []string{}}
	for _, c := range cases {
		if !listHas(names, c.Name) {
			continue
		}
		for _, run := range c.ByModel {
			if run.Model != model {
				continue
			}
			m.Runs += run.Runs
			m.Passed += run.Passed
			if !run.Held {
				m.BelowBar = append(m.BelowBar, c.Name)
			}
		}
	}
	if m.Runs == 0 {
		return nil
	}
	m.Reliability = float64(m.Passed) / float64(m.Runs)
	sort.Strings(m.BelowBar)
	return &m
}

// modelsThatRan reads the model names out of the committed verdicts, sorted.
// Derived from the records rather than kept as a list here: a page that carried
// its own roster would keep publishing a model whose records were deleted, and
// would silently omit one somebody added.
func modelsThatRan(cases []caseRow) []string {
	seen := map[string]bool{}
	for _, c := range cases {
		for _, run := range c.ByModel {
			seen[run.Model] = true
		}
	}
	out := make([]string, 0, len(seen))
	for model := range seen {
		out = append(out, model)
	}
	sort.Strings(out)
	return out
}

// summariseModel folds one model's whole lane for the page's top table.
func summariseModel(cases []caseRow, model string) modelCoverage {
	summary := modelCoverage{Model: model, BelowBar: []string{}}
	for _, c := range cases {
		for _, run := range c.ByModel {
			if run.Model != model {
				continue
			}
			summary.CasesRecorded++
			summary.Runs += run.Runs
			summary.Passed += run.Passed
			if run.Held {
				summary.CasesHeld++
				continue
			}
			summary.CasesBelowBar++
			summary.BelowBar = append(summary.BelowBar, c.Name)
		}
	}
	if summary.Runs > 0 {
		summary.Reliability = float64(summary.Passed) / float64(summary.Runs)
	}
	sort.Strings(summary.BelowBar)
	return summary
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
	writeCoverageSurfaces(&p, r)
	writeCoverageCases(&p, r)
	writeCoverageCriteria(&p, r)
	writeCoverageReliable(&p, r)
	writeCoverageFailing(&p, r)
	writeCoverageJudges(&p, r)
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
				link, criteria, joinOrDash(c.Requires))
			continue
		}
		for _, run := range c.ByModel {
			result := "**FAIL**"
			if run.Held {
				result = "pass"
			}
			fmt.Fprintf(p, "| %s | `%s` | %s | %d/%d | %d | %s | %s |\n",
				link, run.Model, result, run.Passed, run.Runs, run.PassAt,
				criteria, joinOrDash(c.Requires))
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

// writeCoverageUndriven is the third question, and the one this page exists
// for: what has nobody written a case for. Ordered by what each costs, because
// that is the bill being paid for the untried thing.
func writeCoverageUndriven(p *strings.Builder, r mcpToolCoverage) {
	p.WriteString("## 3. What no use case requires\n\n")
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
		if row.Driven {
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
