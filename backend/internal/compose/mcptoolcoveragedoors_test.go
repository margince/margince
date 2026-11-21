// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The two job tables that sit beside the main one: the same scenarios run
// through the neutral harness, and through the REST door. Each answers a
// different question, so neither shares a column with a CLI result.

import (
	"fmt"
	"strings"
	"testing"
)

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
		"as the tables above.\n\n")
	p.WriteString("| Assistant | How we connected it | Jobs tried | Reliable on |\n|---|---|---:|---:|\n")
	for _, m := range r.REST.Models {
		fmt.Fprintf(p, "| `%s` | %s, REST | %d of %d | %d |\n", m.Name, m.Route, m.CasesRecorded, r.Totals.Cases, m.CasesHeld)
	}
	p.WriteString("\n")
	writeJobTable(p, r.REST.Cases, r.REST.Models, r.Criteria)
}
