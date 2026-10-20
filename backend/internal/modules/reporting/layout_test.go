// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package reporting

import (
	"errors"
	"slices"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

func card(id string, kind crmcontracts.ReportingBlockKind, metric crmcontracts.ReportingMetricID) crmcontracts.ReportingCard {
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
