// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package reporting

import (
	"errors"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

func card(id string, kind crmcontracts.ReportingCardKind, metric crmcontracts.ReportingMetricID) crmcontracts.ReportingCard {
	out := crmcontracts.ReportingCard{Id: id, Kind: kind, Width: crmcontracts.ReportingCardWidthHalf}
	if metric != "" {
		out.Metric = &metric
	}
	return out
}

// Two selected metrics that both have a trend chart, and one that has a stage
// chart.
var layoutMetrics = map[crmcontracts.ReportingMetricID][]crmcontracts.ReportingBlockKind{
	"bookings_won":  {"bookings_trend", "owner_attainment", "metric_reading"},
	"open_pipeline": {"bookings_trend", "stage_distribution"},
}

// Blocks follow the layout: each kind once, where the layout first names it.
func TestALayoutDecidesTheBlocks(t *testing.T) {
	layout := []crmcontracts.ReportingSection{
		{Cards: []crmcontracts.ReportingCard{card("a", "stage_distribution", ""), card("b", "bookings_trend", "bookings_won")}},
		{Cards: []crmcontracts.ReportingCard{card("c", "bookings_trend", "open_pipeline")}},
	}
	selection := crmcontracts.ReportingSelection{Blocks: []crmcontracts.ReportingBlockKind{"owner_attainment"}, Layout: &layout}
	deriveLayoutBlocks(&selection)
	if want := []crmcontracts.ReportingBlockKind{"stage_distribution", "bookings_trend"}; !slices.Equal(selection.Blocks, want) {
		t.Errorf("blocks %v, want %v", selection.Blocks, want)
	}
	if err := validateLayout(layout, layoutMetrics); err != nil {
		t.Errorf("one kind placed for two metrics was refused: %v", err)
	}
}

func TestALayoutRefusesWhatItCannotDraw(t *testing.T) {
	long := strings.Repeat("t", 121)
	many := make([]crmcontracts.ReportingCard, 21)
	for i := range many {
		many[i] = card(strings.Repeat("x", i+1), "bookings_trend", "")
	}
	cases := []struct {
		name   string
		layout []crmcontracts.ReportingSection
	}{
		{"a long title", []crmcontracts.ReportingSection{{Title: &long}}},
		{"nine sections", make([]crmcontracts.ReportingSection, 9)},
		{"twenty-one cards", []crmcontracts.ReportingSection{{Cards: many}}},
		{"a repeated id", []crmcontracts.ReportingSection{{Cards: []crmcontracts.ReportingCard{card("a", "bookings_trend", "bookings_won"), card("a", "stage_distribution", "")}}}},
		{"an id outside the pattern", []crmcontracts.ReportingSection{{Cards: []crmcontracts.ReportingCard{card("Card 1", "bookings_trend", "")}}}},
		{"a kind no metric has", []crmcontracts.ReportingSection{{Cards: []crmcontracts.ReportingCard{card("a", "sdr_outcomes", "")}}}},
		{"a metric without that kind", []crmcontracts.ReportingSection{{Cards: []crmcontracts.ReportingCard{card("a", "stage_distribution", "bookings_won")}}}},
		{"the same chart twice", []crmcontracts.ReportingSection{{Cards: []crmcontracts.ReportingCard{card("a", "bookings_trend", "bookings_won"), card("b", "bookings_trend", "bookings_won")}}}},
		{"the metric details table", []crmcontracts.ReportingSection{{Cards: []crmcontracts.ReportingCard{card("a", "metric_reading", "")}}}},
		{"one metric's owner attainment", []crmcontracts.ReportingSection{{Cards: []crmcontracts.ReportingCard{card("a", "owner_attainment", "bookings_won")}}}},
		{"a question card with no question", []crmcontracts.ReportingSection{{Cards: []crmcontracts.ReportingCard{card("q", "question", "")}}}},
		{"a chart card carrying a question", []crmcontracts.ReportingSection{{Cards: []crmcontracts.ReportingCard{withQuestion(card("q", "stage_distribution", ""), wonByOwner())}}}},
		{"a question with no rows", []crmcontracts.ReportingSection{{Cards: []crmcontracts.ReportingCard{withQuestion(card("q", "question", ""), noRows())}}}},
		{"a question with no measure", []crmcontracts.ReportingSection{{Cards: []crmcontracts.ReportingCard{withQuestion(card("q", "question", ""), crmcontracts.ReportingCardQuestion{Entity: "deals-by-stage"})}}}},
		{"a kind for every metric beside one metric's", []crmcontracts.ReportingSection{{Cards: []crmcontracts.ReportingCard{card("a", "bookings_trend", "bookings_won"), card("b", "bookings_trend", "")}}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := validateLayout(c.layout, layoutMetrics)
			if refusal, ok := errors.AsType[*fieldRefusal](err); !ok || refusal.field != "selection.layout" {
				t.Errorf("answered %v, want a refusal naming selection.layout", err)
			}
		})
	}
}

func wonByOwner() crmcontracts.ReportingCardQuestion {
	return crmcontracts.ReportingCardQuestion{
		Entity:   "deals-by-stage",
		GroupBy:  &[]string{"owner_id"},
		Measures: []crmcontracts.AnalyticsMeasure{{Fn: "count"}},
	}
}

func noRows() crmcontracts.ReportingCardQuestion {
	q := wonByOwner()
	zero := 0
	q.Limit = &zero
	return q
}

func withQuestion(c crmcontracts.ReportingCard, q crmcontracts.ReportingCardQuestion) crmcontracts.ReportingCard {
	c.Question = &q
	return c
}

// A question card stands beside the charts and adds no block: the evaluation
// draws charts, and the page asks the question for each reader.
func TestAQuestionCardAddsNoBlock(t *testing.T) {
	layout := []crmcontracts.ReportingSection{{Cards: []crmcontracts.ReportingCard{
		card("a", "stage_distribution", ""),
		withQuestion(card("q", "question", ""), wonByOwner()),
	}}}
	selection := crmcontracts.ReportingSelection{Layout: &layout}
	deriveLayoutBlocks(&selection)
	if want := []crmcontracts.ReportingBlockKind{"stage_distribution"}; !slices.Equal(selection.Blocks, want) {
		t.Errorf("blocks %v, want %v", selection.Blocks, want)
	}
	if err := validateLayout(layout, layoutMetrics); err != nil {
		t.Errorf("a well-formed question card was refused: %v", err)
	}
}

// enumOf reads one string enum out of crm.yaml as it is written there: the
// schema's `- value` lines up to the next schema.
func enumOf(t *testing.T, contract, schema string) []string {
	t.Helper()
	block := regexp.MustCompile(`(?s)\n    ` + schema + `:\n(.*?)\n    [A-Za-z]+:\n`).FindStringSubmatch(contract)
	if block == nil {
		t.Fatalf("the contract has no %s schema", schema)
	}
	var values []string
	for _, line := range regexp.MustCompile(`(?m)^      - ([a-z_]+)$`).FindAllStringSubmatch(block[1], -1) {
		values = append(values, line[1])
	}
	return values
}

// The card kinds are the chart kinds and question, declared twice in crm.yaml
// because a question is not a block. This reads both declarations, so a chart
// kind added to one and not the other fails here.
func TestTheCardKindsAreTheChartKindsAndQuestion(t *testing.T) {
	raw, err := os.ReadFile("../../../api/crm.yaml")
	if err != nil {
		t.Fatalf("reading the contract: %v", err)
	}
	contract := "\n" + string(raw)
	blocks := enumOf(t, contract, "ReportingBlockKind")
	cards := enumOf(t, contract, "ReportingCardKind")
	if len(blocks) == 0 {
		t.Fatal("the contract lists no chart kinds, so the comparison proved nothing")
	}
	if want := append(slices.Clone(blocks), string(questionKind)); !slices.Equal(cards, want) {
		t.Errorf("card kinds %v, want the chart kinds and question %v", cards, want)
	}
}
