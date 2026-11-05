// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"slices"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/analyticsquery"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// questionEdition is an edition with two question cards, the second placed
// first on the page, and kept answers stored in card-id order.
func questionEdition(alias string) crmcontracts.ReportingEdition {
	groupBy := []string{"company_id"}
	byCompany := crmcontracts.ReportingCardQuestion{Entity: "open-deals-per-company", GroupBy: &groupBy, Measures: []crmcontracts.AnalyticsMeasure{{Fn: "count", As: &alias}}}
	layout := []crmcontracts.ReportingSection{{Cards: []crmcontracts.ReportingCard{
		{Id: "zz-later-id", Kind: "question", Question: &byCompany, Width: crmcontracts.ReportingCardWidthFull},
		{Id: "aa-earlier-id", Kind: "question", Question: &byCompany, Width: crmcontracts.ReportingCardWidthFull},
	}}}
	labels := crmcontracts.AnalyticsIdLabels{"company_id": {"c-1": "Weber GmbH"}}
	answer := crmcontracts.AnalyticsAnswer{
		Columns: []string{"company_id", alias},
		Rows:    []map[string]any{{"company_id": "c-1", alias: float64(7)}, {"company_id": nil, alias: nil, "_withheld": true}},
		Labels:  &labels,
	}
	reason := crmcontracts.ReportingEditionQuestionReasonTooManyRecords
	questions := []crmcontracts.ReportingEditionQuestion{
		{CardId: "aa-earlier-id", Status: crmcontracts.ReportingEditionQuestionStatusNotKept, Reason: &reason},
		{CardId: "zz-later-id", Status: crmcontracts.ReportingEditionQuestionStatusKept, Answer: &answer},
	}
	edition := crmcontracts.ReportingEdition{Questions: &questions}
	edition.Evaluation.Selection.Layout = &layout
	edition.Evaluation.Context.EvaluatedAt = time.Date(2026, 9, 24, 13, 0, 0, 0, time.UTC)
	return edition
}

// The kept answers follow the page, not the order they are stored in, and a
// row the floor withheld carries no value.
func TestAnEditionsQuestionRowsFollowThePage(t *testing.T) {
	rows := questionCSVRows(questionEdition("deals"))
	var got [][]string
	for _, row := range rows {
		got = append(got, []string{row[1], row[2], row[3], row[9], row[10]})
	}
	want := [][]string{
		{"question:zz-later-id", "Weber GmbH", "7", "deals", "ok"},
		{"question:zz-later-id", "", "", "", "withheld"},
		{"question:aa-earlier-id", "", "", "", "not_kept:too_many_records"},
	}
	if !slices.EqualFunc(got, want, slices.Equal) {
		t.Fatalf("question rows %q, want %q", got, want)
	}
}

// A measure's alias is typed by whoever asked the question, so a spreadsheet
// must not read one as a formula.
func TestAnEditionExportGuardsAQuestionsAlias(t *testing.T) {
	rows := questionCSVRows(questionEdition("=HYPERLINK(1)"))
	if context := rows[0][9]; context != "'=HYPERLINK(1)" {
		t.Fatalf("the alias reached the file as %q, want it guarded", context)
	}
}

// A kept money measure is written with its unit and its one currency. Without
// a single currency it has no value and says why.
func TestAKeptMoneyMeasureStatesItsUnitAndCurrency(t *testing.T) {
	native := answerMoney{spec: reportSpec{nativeMeasures: nativeMoney(fieldAmountMinor)}, base: "EUR"}
	noBase := answerMoney{spec: native.spec}
	sum := func(field string) analyticsquery.Measure {
		return analyticsquery.Measure{Fn: analyticsquery.Sum, Field: field}
	}
	for name, tc := range map[string]struct {
		money   answerMoney
		measure analyticsquery.Measure
		row     map[string]any
		want    [3]string
	}{
		"a native amount in its row's currency":    {native, sum(fieldAmountMinor), map[string]any{fieldCurrency: "USD"}, [3]string{"minor_units", "USD", "ok"}},
		"a native amount over several currencies":  {native, sum(fieldAmountMinor), map[string]any{}, [3]string{"", "", "mixed_currency"}},
		"a converted amount in the base currency":  {native, sum(fieldAmountBaseMinor), map[string]any{}, [3]string{"minor_units", "EUR", "ok"}},
		"a converted amount with no base currency": {noBase, sum(fieldAmountBaseMinor), map[string]any{}, [3]string{"", "", "no_base_currency"}},
		"a count of a money field":                 {native, analyticsquery.Measure{Fn: analyticsquery.CountAll, Field: fieldAmountMinor}, map[string]any{}, [3]string{"", "", "ok"}},
	} {
		t.Run(name, func(t *testing.T) {
			unit, currency, coverage := tc.money.of(tc.measure, tc.row)
			if got := [3]string{unit, currency, coverage}; got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// Retention and erasure leave an edition with no answers at all. That differs
// from answers never kept, so the file says nothing about its cards.
func TestAnEditionWhoseAnswersWereRemovedWritesNoQuestionRows(t *testing.T) {
	edition := questionEdition("deals")
	edition.Questions = nil
	if rows := questionCSVRows(edition); len(rows) != 0 {
		t.Fatalf("an edition with its answers removed wrote %q", rows)
	}
}
