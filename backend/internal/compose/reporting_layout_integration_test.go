// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"slices"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A saved layout keeps its sections and widths, decides the report's blocks
// whatever the client sent, and reaches the reader through the evaluation.
func TestASavedReportKeepsItsLayout(t *testing.T) {
	f := reportingBusiness(t)
	title := "Pipeline"
	layout := []crmcontracts.ReportingSection{
		{Title: &title, Cards: []crmcontracts.ReportingCard{
			{Id: "stages", Kind: "stage_distribution", Width: crmcontracts.ReportingCardWidthHalf},
			{Id: "age", Kind: "stage_age", Width: crmcontracts.ReportingCardWidthHalf},
		}},
		{Cards: []crmcontracts.ReportingCard{
			{Id: "trend", Kind: "bookings_trend", Width: crmcontracts.ReportingCardWidthFull},
		}},
	}
	selection := f.selection()
	selection.Layout = &layout
	created, err := f.service.CreateReport(f.human, crmcontracts.ReportingReportInput{
		Name: "Laid out", Audience: "private", Selection: selection,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []crmcontracts.ReportingBlockKind{"stage_distribution", "stage_age", "bookings_trend"}
	if !slices.Equal(created.Selection.Blocks, want) {
		t.Errorf("blocks %v, want %v from the layout", created.Selection.Blocks, want)
	}

	evaluation, err := f.service.EvaluateReport(f.human, ids.UUID(created.Id), nil)
	if err != nil {
		t.Fatal(err)
	}
	got := evaluation.Selection.Layout
	if got == nil || len(*got) != 2 || (*got)[0].Title == nil || *(*got)[0].Title != title ||
		(*got)[0].Cards[1].Width != crmcontracts.ReportingCardWidthHalf {
		t.Fatalf("the evaluation carries layout %+v, want the two sections as saved", got)
	}
	for _, chart := range evaluation.Charts {
		if chart.Kind == "owner_attainment" {
			t.Errorf("the evaluation drew owner_attainment, which the layout left out")
		}
	}
}

// A question card is kept with its question and adds no chart to the
// evaluation. It reaches the reader in the evaluation's layout, for the page
// to ask.
func TestASavedReportKeepsItsQuestionCard(t *testing.T) {
	f := reportingBusiness(t)
	question := crmcontracts.ReportingCardQuestion{
		Entity:   "deals-by-stage",
		GroupBy:  &[]string{"owner_id"},
		Measures: []crmcontracts.AnalyticsMeasure{{Fn: "count"}},
	}
	layout := []crmcontracts.ReportingSection{{Cards: []crmcontracts.ReportingCard{
		{Id: "stages", Kind: "stage_distribution", Width: crmcontracts.ReportingCardWidthHalf},
		{Id: "by-owner", Kind: "question", Question: &question, Width: crmcontracts.ReportingCardWidthHalf},
	}}}
	selection := f.selection()
	selection.Layout = &layout
	created, err := f.service.CreateReport(f.human, crmcontracts.ReportingReportInput{
		Name: "With a question", Audience: "private", Selection: selection,
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := []crmcontracts.ReportingBlockKind{"stage_distribution"}; !slices.Equal(created.Selection.Blocks, want) {
		t.Errorf("blocks %v, want %v: a question is not a chart", created.Selection.Blocks, want)
	}
	evaluation, err := f.service.EvaluateReport(f.human, ids.UUID(created.Id), nil)
	if err != nil {
		t.Fatal(err)
	}
	cards := (*evaluation.Selection.Layout)[0].Cards
	if cards[1].Question == nil || cards[1].Question.Entity != "deals-by-stage" {
		t.Fatalf("the evaluation's layout carries %+v, want the question card as saved", cards[1])
	}
}
