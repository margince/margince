// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/analyticsquery"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/reporting"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// saveForecastRun keeps a question as its asker saves one, through the real
// writer.
func saveForecastRun(ctx context.Context, t *testing.T, e *forecastEnv, q analyticsquery.Query) ids.UUID {
	t.Helper()
	var id ids.UUID
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		answer, err := RunAnalyticsQuery(ctx, tx, q, analyticsquery.Floor(1))
		if err != nil {
			return err
		}
		id, err = SaveReportRun(ctx, tx, q, answer, analyticsquery.Floor(1))
		return err
	}); err != nil {
		t.Fatalf("saving %+v: %v", q, err)
	}
	return id
}

// savedRunCount re-asks a saved question and answers its one count.
func savedRunCount(ctx context.Context, t *testing.T, e *forecastEnv, run ids.UUID) string {
	t.Helper()
	var read ReportRun
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		var err error
		read, err = ReadReportRun(ctx, tx, run, analyticsquery.Floor(1))
		return err
	}); err != nil {
		t.Fatalf("re-asking the saved question: %v", err)
	}
	return fmt.Sprint(read.Answer.Rows[0]["count"])
}

// boundCount asks a report's bound card and answers its one count.
func boundCount(ctx context.Context, t *testing.T, e *forecastEnv, service *reporting.Service, report crmcontracts.ReportingReport) string {
	t.Helper()
	h := newAnalyticsQueryHandlers(InstallationDB(e.Pool), analyticsquery.Floor(1), newAttentionNames(InstallationDB(e.Pool)))
	h.reportMetrics = service
	recorder := httptest.NewRecorder()
	h.AnswerReportingQuestionCard(recorder, httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx), report.Id, "bound", crmcontracts.AnswerReportingQuestionCardParams{})
	answer := decodeAnswer(t, recorder.Body.String())
	if answer.Answer == nil {
		t.Fatalf("the bound card answered %s", recorder.Body.String())
	}
	return fmt.Sprint(answer.Answer.Rows[0]["count"])
}

func moveFiscalStart(ctx context.Context, t *testing.T, e *forecastEnv, month int) {
	t.Helper()
	if _, err := NewInstallationSettingsFor(e.Pool).UpdateInstallation(ctx, identity.InstallationPatch{FiscalYearStartMonth: &month}); err != nil {
		t.Fatalf("moving the fiscal start to %d: %v", month, err)
	}
}

// A period saved as a calendar year keeps naming that year after the fiscal
// start moves. The move writes down the calendar each saved question was asked
// in. The admin is told how many there are first.
func TestASavedPeriodKeepsItsDatesWhenTheFiscalStartMoves(t *testing.T) {
	e := setupForecast(t)
	ctx := reportingActor(e)
	year := clocktest.Now(t).Year() - 1
	for _, closed := range []string{
		fmt.Sprintf("%d-02-10T12:00:00Z", year),
		fmt.Sprintf("%d-11-10T12:00:00Z", year),
		fmt.Sprintf("%d-02-10T12:00:00Z", year+1),
	} {
		e.seedClosedDeal(t, "Won "+closed, "won", closed, 10000)
	}
	calendarYear := analyticsquery.Query{
		Entity:   "win-loss",
		Measures: []analyticsquery.Measure{{Fn: analyticsquery.CountAll}},
		Filters:  []analyticsquery.Filter{{Field: fieldPeriodYear, Op: analyticsquery.OpEq, Value: fmt.Sprint(year)}},
	}
	saved := []ids.UUID{saveForecastRun(ctx, t, e, calendarYear), saveForecastRun(ctx, t, e, calendarYear)}

	var impact crmcontracts.InstallationFiscalYearImpact
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		var err error
		impact, err = fiscalYearImpact(ctx, tx)
		return err
	}); err != nil || impact.SavedQuestions != 2 {
		t.Fatalf("before the move the admin is told %+v (%v), want the 2 saved questions that filter on a period", impact, err)
	}

	service := newReportingService(e.Pool, func() time.Time { return clocktest.Now(t) })
	report, err := service.CreateReport(ctx, reportInput(boundCard(saved[0]), workspaceScope))
	if err != nil {
		t.Fatalf("binding the saved period: %v", err)
	}

	moveFiscalStart(ctx, t, e, 4)
	if got := boundCount(ctx, t, e, service, report); got != "2" {
		t.Errorf("the bound card counts %s deals in %d after the start moved to April, want the 2 it named when it was placed", got, year)
	}
	for _, run := range saved {
		if got := savedRunCount(ctx, t, e, run); got != "2" {
			t.Errorf("a saved question counts %s deals in %d after the start moved to April, want the 2 it named under January", got, year)
		}
	}
	fiscal := calendarYear
	fiscal.Filters = []analyticsquery.Filter{{Field: fieldPeriodYear, Op: analyticsquery.OpEq, Value: fmt.Sprintf("FY%d/%02d", year, (year+1)%100)}}
	if got := savedRunCount(ctx, t, e, saveForecastRun(ctx, t, e, fiscal)); got != "2" {
		t.Errorf("a fiscal year asked under April counts %s deals, want the November and the February one", got)
	}
}

// A sent edition keeps the calendar it was captured under. A live read of the
// same report follows the setting and says which start it read.
func TestAFrozenEditionKeepsItsCalendarWhenTheFiscalStartMoves(t *testing.T) {
	q := newQuestionEditions(t)
	q.keepForAYear(t)
	edition := q.send(t)
	before := q.read(q.human, t, ids.UUID(edition.Id))

	moveFiscalStart(q.human, t, q.e, 7)
	after := q.read(q.human, t, ids.UUID(edition.Id))
	was, now := before.Evaluation.Context.Interval, after.Evaluation.Context.Interval
	if !was.StartAt.Equal(now.StartAt) || !was.EndAt.Equal(now.EndAt) ||
		after.Evaluation.Context.FiscalYearStartMonth == nil || *after.Evaluation.Context.FiscalYearStartMonth != 1 {
		t.Fatalf("the edition reads %+v after the move, want the interval and January start it was captured under", after.Evaluation.Context)
	}
	live, err := q.service.EvaluateReport(q.human, ids.UUID(q.report.Id), nil)
	if err != nil || live.Context.FiscalYearStartMonth == nil || *live.Context.FiscalYearStartMonth != 7 {
		t.Fatalf("a live read after the move says %+v (%v), want the July start", live.Context.FiscalYearStartMonth, err)
	}
}

// A scheduled edition asks a bound card through the same resolver as the
// page. The filter its saved question carries holds in the kept answer. A
// native money sum stays answerable, because the currency filter is kept.
func TestASentEditionAsksABoundCardWithItsFilters(t *testing.T) {
	q := newQuestionEditions(t)
	q.keepForAYear(t)
	for range 3 {
		q.e.seedOpenDeal(t, "Unpriced", 20, &q.e.Rep1, nil, nil)
	}
	run := saveForecastRun(q.human, t, q.e, analyticsquery.Query{
		Entity:   "open-deals-per-company",
		Measures: []analyticsquery.Measure{{Fn: analyticsquery.CountAll}, {Fn: analyticsquery.Sum, Field: fieldAmountMinor}},
		Filters:  []analyticsquery.Filter{{Field: fieldCurrency, Op: analyticsquery.OpEq, Value: "EUR"}},
	})
	saved := openapi_types.UUID(run)
	layout := []crmcontracts.ReportingSection{{Cards: []crmcontracts.ReportingCard{
		{Id: "trend", Kind: "bookings_trend", Width: crmcontracts.ReportingCardWidthFull},
		{Id: "open-deals", Kind: "question", Width: crmcontracts.ReportingCardWidthFull, Question: &crmcontracts.ReportingCardQuestion{
			Entity: "open-deals-per-company", Measures: []crmcontracts.AnalyticsMeasure{{Fn: "count"}}, SavedQuestionId: &saved,
		}},
	}}}
	input := crmcontracts.ReportingReportInput{Name: q.report.Name, Audience: "workspace", Selection: q.report.Selection}
	input.Selection.Layout = &layout
	updated, err := q.service.UpdateReport(q.human, ids.UUID(q.report.Id), q.report.Version, input)
	if err != nil {
		t.Fatalf("binding the card: %v", err)
	}
	q.report = updated

	kept := onlyQuestion(t, q.send(t))
	if kept.Status != crmcontracts.ReportingEditionQuestionStatusKept || kept.Answer == nil {
		t.Fatalf("the bound card reads %+v in the edition, want its kept answer", kept)
	}
	if got := fmt.Sprint(kept.Answer.Rows[0]["count"]); got != "7" {
		t.Fatalf("the edition kept a count of %s, want the 7 priced deals its filter names, not all 10", got)
	}
}
