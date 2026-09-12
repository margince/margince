// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// What an agent's tool menu COSTS, derived rather than asserted.
//
// It lives in the test lane, like mcp-info's renderers beside it, because
// nothing in the product reads these numbers — they are published for the
// contact deciding whether to attach a tool. A derivation with no production
// caller does not belong in the binary.
//
// The listing rides in every step of a tool-fed window, so attaching a tool
// spends prompt on every turn of every run for as long as the agent exists.
// An author adding a tool needs to see what it costs THEIR agent — both in
// prompt and in that agent's ability to pick the right tool — and a number
// measured across the whole catalog, which no agent is ever offered, answers
// neither question.
//
// These are the three things the published page reports, each derived from an
// artefact already in this tree so none of them can be maintained into being
// wrong: the rendered listing (the registry), the cross-references (the tool
// copy), and the mis-selection weight (the certification corpus).

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// toolNameInProse matches an identifier shaped like a tool name. It is
// deliberately loose: what makes a match a REFERENCE is that the registry
// answers to it, which the callers check. A pattern tight enough to recognise
// tool names on its own would be a second, drifting copy of the catalog.
var toolNameInProse = regexp.MustCompile(`\b[a-z][a-z0-9_]{3,}\b`)

// crossReferences maps each tool to the other registered tools its own
// description names.
//
// The rule is ANY registered tool name appearing in another tool's
// description — NOT a "Use X when …" clause. Several disambiguation sentences
// carry no "Use" at all (catch_me_up_on writes "prep_for_meeting when a meeting
// is about to happen, read_record for the record's own stored fields"), so a
// use-clause pattern would report a sparser graph than the copy actually has.
func crossReferences(specs []mcp.ToolSpec) map[string][]string {
	registered := make(map[string]bool, len(specs))
	for _, spec := range specs {
		registered[spec.Name] = true
	}
	graph := make(map[string][]string, len(specs))
	for _, spec := range specs {
		seen := map[string]bool{}
		for _, match := range toolNameInProse.FindAllString(spec.Description, -1) {
			if match == spec.Name || !registered[match] || seen[match] {
				continue
			}
			seen[match] = true
			graph[spec.Name] = append(graph[spec.Name], match)
		}
		sort.Strings(graph[spec.Name])
	}
	return graph
}

// danglingReferences counts the times an attached tool's own copy points at a
// tool this agent cannot call.
//
// It is a DIAGNOSTIC, never a target. Closing a menu under this relation is
// unaffordable — either shipped agent's seed closes to 30 tools and ~10,500
// tokens, six times its size — and lowering it is not always an improvement:
// adding review_commitments to the sweep raises this count while cutting the
// agent's mis-selection weight almost in half. It tells an author that a run
// may waste a step discovering a refusal, and nothing more.
func danglingReferences(attached []string, graph map[string][]string) []string {
	held := make(map[string]bool, len(attached))
	for _, name := range attached {
		held[name] = true
	}
	var dangling []string
	for _, name := range attached {
		for _, neighbour := range graph[name] {
			if !held[neighbour] {
				dangling = append(dangling, name+" → "+neighbour)
			}
		}
	}
	sort.Strings(dangling)
	return dangling
}

// agentLoopCorpusDir is where the certification scenarios live, relative to
// this package.
const agentLoopCorpusDir = "aicert/corpus/agent_loop"

// wrongReachCensus is how often each tool is named as the WRONG reach across
// the agent_loop certification scenarios, plus what the scan could not read.
//
// A scenario DECLARES its near misses, and that is what is counted. Where it
// declares none the rubric's prose is scanned instead — a mention counts when
// the rubric names a registered tool that is not that scenario's own answer —
// and the scenario is NAMED as read that way, on the page.
//
// The prose scan over-counts, because a rubric quotes the right tool's copy and
// that copy names its neighbours, and it is sensitive to how a rubric is
// phrased. That is why it is the fallback rather than the method.
//
// Skipped names what the scan could not classify. It is reported rather than
// dropped: a census that silently ignored the scenarios it could not read would
// look exactly like one that found nothing to report in them.
type wrongReachCensus struct {
	Counts    map[string]int
	Scenarios int
	Catalog   int
	Skipped   []string
	// Heuristic names the scenarios whose near misses were read out of rubric
	// PROSE because the scenario declares none of its own.
	//
	// Named rather than counted, and named on the published page. A fallback
	// that is silent about which scenarios it still applies to hides exactly
	// the error it exists to shrink: the reader sees one number and cannot tell
	// which part of it was measured and which part guessed.
	Heuristic []string
}

var (
	// The corpus writes an expected step two ways: `answer: read_record` when
	// the step alone is graded, and a nested `answer:` block carrying `step:`
	// when the arguments are graded too. Nine of the twenty-three use the
	// second, and reading only the first would silently drop them — along with
	// the catch_me_up_on/prep_for_meeting confusion this census exists to
	// surface.
	scenarioAnswer       = regexp.MustCompile(`(?m)^\s{2}answer:\s*(\S+)\s*$`)
	scenarioAnswerNested = regexp.MustCompile(`(?m)^\s{2}answer:\s*\n\s{4}step:\s*(\S+)\s*$`)
	scenarioTools        = regexp.MustCompile(`(?m)^\s{2}tools:\s*(\S+)\s*$`)
	scenarioRubric       = regexp.MustCompile(`(?s)\n  rubric:(.*?)\n  bands:`)
)

func readWrongReachCensus(dir string, specs []mcp.ToolSpec) (wrongReachCensus, error) {
	registered := make(map[string]bool, len(specs))
	for _, spec := range specs {
		registered[spec.Name] = true
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return wrongReachCensus{}, err
	}
	census := wrongReachCensus{Counts: map[string]int{}}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}
		census.Scenarios++
		body, readErr := os.ReadFile(filepath.Join(dir, entry.Name()))
		if readErr != nil {
			return wrongReachCensus{}, readErr
		}
		text := string(body)
		if tools := scenarioTools.FindStringSubmatch(text); len(tools) == 2 && tools[1] == "catalog" {
			census.Catalog++
		}
		rubric := scenarioRubric.FindStringSubmatch(text)
		if len(rubric) != 2 {
			census.Skipped = append(census.Skipped,
				entry.Name()+" (no rubric block this scan could read)")
			continue
		}
		// A scenario whose expected step this scan cannot read CANNOT be
		// counted: with no answer to subtract, the tool the scenario exists to
		// reward is counted as a wrong reach, and the census would report the
		// right answer as the temptation. Such a scenario is NAMED rather than
		// absorbed — a scan that quietly dropped one would publish a smaller,
		// cleaner number for a reason nobody could see.
		answer := ""
		for _, pattern := range []*regexp.Regexp{scenarioAnswer, scenarioAnswerNested} {
			if got := pattern.FindStringSubmatch(text); len(got) == 2 {
				answer = got[1]
				break
			}
		}
		if answer == "" {
			census.Skipped = append(census.Skipped,
				entry.Name()+" (this scan could not read its expected step, so a wrong reach cannot be told from the right one)")
			continue
		}
		named := declaredNearMisses(body, registered, answer)
		if named == nil {
			census.Heuristic = append(census.Heuristic, entry.Name())
			named = map[string]bool{}
			for _, match := range toolNameInProse.FindAllString(rubric[1], -1) {
				if registered[match] && match != answer {
					named[match] = true
				}
			}
		}
		for name := range named {
			census.Counts[name]++
		}
	}
	sort.Strings(census.Skipped)
	sort.Strings(census.Heuristic)
	return census, nil
}

// declaredNearMisses is the scenario's own list, or nil when it declares none.
//
// Nil and empty are different answers and the caller acts on the difference: a
// scenario that declares none is read by the prose fallback and said to be,
// while one declaring an empty list has said its goal has no plausible wrong
// reach — a claim, not an absence.
//
// Decoded here rather than through aicert.LoadScenarioFile because aicert
// imports this package, so a test in it cannot import aicert back. The key is
// aicert.Expectations' own `near_misses` tag and this is a MIRROR of it: rename
// the field there and every scenario falls to the prose fallback, which
// TestTheWrongReachCensusReadsWhatTheScenariosDeclare below fails on.
//
// The answer is excluded even when a scenario names it, because a list holding
// the tool the scenario exists to reward would count the right answer as the
// temptation — the failure the skipped-scenario guard above is about.
func declaredNearMisses(body []byte, registered map[string]bool, answer string) map[string]bool {
	var declared struct {
		Expect struct {
			NearMisses []string `yaml:"near_misses"`
		} `yaml:"expect"`
	}
	if err := yaml.Unmarshal(body, &declared); err != nil || declared.Expect.NearMisses == nil {
		return nil
	}
	named := map[string]bool{}
	for _, tool := range declared.Expect.NearMisses {
		if registered[tool] && tool != answer {
			named[tool] = true
		}
	}
	return named
}

// temptationWeight sums the corpus's wrong-reach counts over an agent's
// attached tools.
//
// It orders which tools cause trouble ON THIS SURFACE; it is NOT a prediction
// about one agent. Each count was measured under a different scenario's goal,
// so summing them over a menu whose agent has one fixed goal borrows precision
// the number does not have. The page carries that caveat next to the figure.
func temptationWeight(attached []string, census wrongReachCensus) int {
	weight := 0
	for _, name := range attached {
		weight += census.Counts[name]
	}
	return weight
}
