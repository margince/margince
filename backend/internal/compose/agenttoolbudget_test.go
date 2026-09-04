// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// docs/reference/agent-tool-budget.{json,md} is what an agent's tool menu
// costs, published where somebody deciding whether to attach a tool will see it
// BEFORE a gate tells them. It is rendered from the tree — the registry, the
// tool copy, the certification corpus — so it states what the software does,
// not what somebody believed on the day they wrote it down.
//
// The JSON is the machine half and the page renders from it, the split
// mcp-info.{json,md} already uses, so a later dashboard reads the payload and
// never scrapes prose.

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/agents/runner"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

var (
	agentToolBudgetDoc  = filepath.Join("..", "..", "..", "docs", "reference", "agent-tool-budget.json")
	agentToolBudgetPage = filepath.Join("..", "..", "..", "docs", "reference", "agent-tool-budget.md")
)

// One flag rewrites BOTH, so a regeneration never leaves the page describing a
// surface the payload beside it no longer carries.
var updateAgentToolBudget = flag.Bool("update-agent-tool-budget", false,
	"rewrite docs/reference/agent-tool-budget.{json,md} from the declared agents")

const agentToolBudgetCommand = "go test ./internal/compose/ -run TestTheAgentToolBudgetIsPublished -update-agent-tool-budget"

type agentToolBudget struct {
	Note          string           `json:"note"`
	PromptCeiling int              `json:"prompt_token_ceiling"`
	AgentBudget   int              `json:"per_agent_listing_budget"`
	CatalogFloor  int              `json:"whole_catalog_floor"`
	Catalog       catalogTotals    `json:"catalog"`
	Agents        []agentBudgetRow `json:"agents"`
	ToolCost      []toolCostRow    `json:"tool_cost"`
	WrongReach    []wrongReachRow  `json:"corpus_wrong_reach"`
	Corpus        corpusProvenance `json:"corpus"`
}

type catalogTotals struct {
	Tools int `json:"tools"`
	// Frame is what the system prompt costs before any tool is listed. It is
	// here because a sentence moved OUT of the per-tool schemas and into the
	// frame is a saving of (tools x sentence) against a cost of (1 x sentence),
	// and only the first half used to be measured: the catalog floor holds the
	// LISTING alone, so a frame that grew a paragraph spent it on every run of
	// every agent with nothing published and no assertion anywhere.
	Frame  int `json:"system_frame_tokens"`
	Tokens int `json:"tokens"`
	Median int `json:"median_tool_tokens"`
	Mean   int `json:"mean_tool_tokens"`
}

type agentBudgetRow struct {
	Name       string   `json:"name"`
	Goal       string   `json:"goal"`
	Tools      []string `json:"tools"`
	Tokens     int      `json:"tokens"`
	PercentOf  int      `json:"percent_of_ceiling"`
	Headroom   int      `json:"headroom_tokens"`
	Dangling   []string `json:"dangling_cross_references"`
	Temptation int      `json:"temptation_weight"`
}

type toolCostRow struct {
	Name   string `json:"name"`
	Tokens int    `json:"tokens"`
}

type wrongReachRow struct {
	Name      string `json:"name"`
	Scenarios int    `json:"scenarios_naming_it_as_the_wrong_reach"`
}

type corpusProvenance struct {
	Scenarios       int      `json:"scenarios"`
	OfferingCatalog int      `json:"offering_the_whole_catalog"`
	Skipped         []string `json:"skipped_by_the_scan"`
}

func TestTheAgentToolBudgetIsPublished(t *testing.T) {
	payload := renderAgentToolBudget(t)
	encoded, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		t.Fatalf("encoding the budget: %v", err)
	}
	encoded = append(encoded, '\n')
	syncAgentToolBudget(t, agentToolBudgetDoc, encoded)
	syncAgentToolBudget(t, agentToolBudgetPage, renderAgentToolBudgetPage(payload))
}

func renderAgentToolBudget(t *testing.T) agentToolBudget {
	t.Helper()
	specs := servedSurface(t).Specs()
	graph := crossReferences(specs)
	census, err := readWrongReachCensus(agentLoopCorpusDir, specs)
	if err != nil {
		t.Fatalf("reading the certification corpus at %s: %v", agentLoopCorpusDir, err)
	}

	cost := make([]toolCostRow, 0, len(specs))
	sum := 0
	for _, spec := range specs {
		tokens := len(runner.ToolListing([]mcp.ToolSpec{spec})) / 4
		cost = append(cost, toolCostRow{Name: spec.Name, Tokens: tokens})
		sum += tokens
	}
	sort.Slice(cost, func(i, j int) bool {
		if cost[i].Tokens != cost[j].Tokens {
			return cost[i].Tokens > cost[j].Tokens
		}
		return cost[i].Name < cost[j].Name
	})
	ordered := make([]int, 0, len(cost))
	for _, row := range cost {
		ordered = append(ordered, row.Tokens)
	}
	sort.Ints(ordered)

	budget := runner.PromptTokenCeiling * listingBudgetNumerator / listingBudgetDenominator
	var rows []agentBudgetRow
	for _, spec := range mustScheduledAgents() {
		// specsNamed rather than an inline lookup: it REFUSES a name with no
		// registered tool behind it, so a menu that measures small because a
		// tool went missing fails here instead of publishing the same number
		// for the opposite reason.
		attached := specsNamed(t, spec.Tools)
		tokens := len(runner.ToolListing(attached)) / 4
		rows = append(rows, agentBudgetRow{
			Name:       spec.Name,
			Goal:       spec.Goal,
			Tools:      spec.Tools,
			Tokens:     tokens,
			PercentOf:  tokens * 100 / runner.PromptTokenCeiling,
			Headroom:   budget - tokens,
			Dangling:   danglingReferences(spec.Tools, graph),
			Temptation: temptationWeight(spec.Tools, census),
		})
	}

	reach := make([]wrongReachRow, 0, len(census.Counts))
	for name, n := range census.Counts {
		reach = append(reach, wrongReachRow{Name: name, Scenarios: n})
	}
	sort.Slice(reach, func(i, j int) bool {
		if reach[i].Scenarios != reach[j].Scenarios {
			return reach[i].Scenarios > reach[j].Scenarios
		}
		return reach[i].Name < reach[j].Name
	})

	return agentToolBudget{
		Note:          agentToolBudgetNote,
		PromptCeiling: runner.PromptTokenCeiling,
		AgentBudget:   budget,
		CatalogFloor:  runner.PromptTokenCeiling * wholeCatalogBudgetNumerator / wholeCatalogBudgetDenominator,
		Catalog: catalogTotals{
			Tools:  len(specs),
			Frame:  runner.SystemFrameTokens(),
			Tokens: len(runner.ToolListing(specs)) / 4,
			Median: medianTokens(ordered),
			Mean:   sum / len(ordered),
		},
		Agents:     rows,
		ToolCost:   cost,
		WrongReach: reach,
		Corpus: corpusProvenance{
			Scenarios:       census.Scenarios,
			OfferingCatalog: census.Catalog,
			Skipped:         census.Skipped,
		},
	}
}

const agentToolBudgetNote = "Generated by `" + agentToolBudgetCommand + "`; do not edit by hand. " +
	"It reports what each SCHEDULED agent's tool listing costs the window it runs in — not the " +
	"whole served catalog, which no agent is ever offered. Token counts use the ~4-bytes-per-token " +
	"estimate the window itself estimates with, over the listing the runner's own renderer produces. " +
	"tool_cost rows are NOT additive: each is one tool rendered alone and divided by four, so " +
	"every row carries its own rounding, while catalog.tokens divides the whole rendered listing once."

// medianTokens is the true median: with an even catalog the upper-middle
// element is not it, and this page is a reference other numbers get checked
// against.
func medianTokens(sorted []int) int {
	mid := len(sorted) / 2
	if len(sorted)%2 == 1 {
		return sorted[mid]
	}
	return (sorted[mid-1] + sorted[mid]) / 2
}

func syncAgentToolBudget(t *testing.T, path string, want []byte) {
	t.Helper()
	if *updateAgentToolBudget {
		if err := os.WriteFile(path, want, 0o644); err != nil {
			t.Fatalf("rewriting %s: %v", path, err)
		}
		return
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the committed artifact %s: %v\nRegenerate with: %s", path, err, agentToolBudgetCommand)
	}
	if bytes.Equal(got, want) {
		return
	}
	// Say WHAT differs. A golden gate that only reports staleness sends the
	// reader to regenerate blind, and when the difference is environmental
	// rather than authored — a machine rendering this differently from CI —
	// regenerating is exactly the wrong move.
	t.Errorf("%s is stale — it no longer matches what the declared agents cost.\n"+
		"Regenerate with: %s\nand commit the result with the change that moved the numbers.\n%s",
		path, agentToolBudgetCommand, firstMCPInfoDifference(string(got), string(want)))
}

// The extraction rules are prose heuristics, so each is held to a case it must
// get right and a case it must not claim. Without these the page would publish
// two numbers whose method nothing checks.
func TestTheCrossReferenceScanReadsTheCopyAndNotItsShape(t *testing.T) {
	specs := []mcp.ToolSpec{
		// No "Use" anywhere: a use-clause pattern would report no edge here,
		// and this is how catch_me_up_on actually names its neighbours.
		{Name: "catch_me_up_on", Description: "prep_for_meeting when a meeting is about to happen, read_record for the stored fields."},
		{Name: "prep_for_meeting", Description: "Assemble what a person needs before they walk in."},
		{Name: "read_record", Description: "Read one record's own stored fields; mentions no other tool."},
	}
	graph := crossReferences(specs)
	if got := graph["catch_me_up_on"]; len(got) != 2 || got[0] != "prep_for_meeting" || got[1] != "read_record" {
		t.Errorf("the scan read %v from a description naming two tools with no \"Use\" clause", got)
	}
	if got := graph["read_record"]; len(got) != 0 {
		t.Errorf("the scan invented %v from a description that names no tool", got)
	}

	if got := danglingReferences([]string{"catch_me_up_on", "read_record"}, graph); len(got) != 1 ||
		got[0] != "catch_me_up_on → prep_for_meeting" {
		t.Errorf("dangling references were %v; the attached pair points at exactly one tool it cannot call", got)
	}
	if got := danglingReferences([]string{"catch_me_up_on", "read_record", "prep_for_meeting"}, graph); len(got) != 0 {
		t.Errorf("a closed menu reported %v dangling", got)
	}
}

// The census must be derived from the corpus on disk, and it must report what
// it could not read rather than dropping it — a scan that silently skipped a
// scenario would look exactly like one that found nothing to report in it.
func TestTheWrongReachCensusIsReadFromTheCorpusAndNamesWhatItSkipped(t *testing.T) {
	specs := servedSurface(t).Specs()
	census, err := readWrongReachCensus(agentLoopCorpusDir, specs)
	if err != nil {
		t.Fatalf("reading the corpus: %v", err)
	}
	if census.Scenarios == 0 {
		t.Fatal("the census read no scenarios — it is looking at the wrong directory, " +
			"which reads exactly like a corpus with nothing in it")
	}
	if census.Catalog == 0 {
		t.Error("no scenario was seen to offer the whole catalog, though the corpus is built on that shape")
	}
	if len(census.Counts) == 0 {
		t.Error("no tool was named as a wrong reach in any rubric, so the census is measuring nothing")
	}
	// A scenario the scan cannot classify must be NAMED, not absorbed.
	for _, skipped := range census.Skipped {
		if !strings.Contains(skipped, ".yaml") {
			t.Errorf("a skipped scenario is recorded as %q, which does not say which file it was", skipped)
		}
	}
	if census.Scenarios < census.Catalog {
		t.Errorf("the census counted %d scenarios offering the catalog out of %d total",
			census.Catalog, census.Scenarios)
	}
}
