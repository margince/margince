// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package salesprogress

import (
	"context"
	"math"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/salesassessment"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/reporting"
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy"
)

// summaryMembers bounds the members a team's summary reads one by one.
const summaryMembers = 12

var summaryWords = struct{ Goal, Closed, Missing, Over, Completion, Planned, Product, Service, Unresolved, GoalMonths langcopy.Phrase }{
	Goal:       langcopy.Phrase{En: "Sales goal", De: "Umsatzziel", Vi: "Mục tiêu doanh số"},
	Closed:     langcopy.Phrase{En: "Closed revenue", De: "Abgeschlossener Umsatz", Vi: "Doanh thu đã chốt"},
	Missing:    langcopy.Phrase{En: "Still missing to the goal", De: "Fehlt noch bis zum Ziel", Vi: "Còn thiếu so với mục tiêu"},
	Over:       langcopy.Phrase{En: "Above the goal", De: "Über dem Ziel", Vi: "Vượt mục tiêu"},
	Completion: langcopy.Phrase{En: "Share of the goal reached", De: "Erreichter Anteil des Ziels", Vi: "Tỷ lệ mục tiêu đã đạt"},
	Planned:    langcopy.Phrase{En: "Open deals expected to close in the period", De: "Offene Deals mit erwartetem Abschluss im Zeitraum", Vi: "Deal đang mở dự kiến chốt trong kỳ"},
	Product:    langcopy.Phrase{En: "Closed revenue from products", De: "Abgeschlossener Umsatz aus Produkten", Vi: "Doanh thu đã chốt từ sản phẩm"},
	Service:    langcopy.Phrase{En: "Closed revenue from services", De: "Abgeschlossener Umsatz aus Dienstleistungen", Vi: "Doanh thu đã chốt từ dịch vụ"},
	Unresolved: langcopy.Phrase{En: "Closed revenue with no sale type", De: "Abgeschlossener Umsatz ohne Verkaufsart", Vi: "Doanh thu đã chốt chưa có loại bán hàng"},
	GoalMonths: langcopy.Phrase{En: "months of the period have a goal", De: "Monate des Zeitraums haben ein Ziel", Vi: "tháng trong kỳ có mục tiêu"},
}

// SummaryInput reads one Progress page as the figures its summary may cite,
// with the key of the evaluation they came from. The page's own read answers
// it, so the summary holds no figure the reader's page does not.
func (h Reading) SummaryInput(ctx context.Context, params crmcontracts.GetSalesProgressParams) (salesassessment.Input, string, error) {
	var progress crmcontracts.SalesProgress
	err := h.DB.TxIsolated(ctx, pgx.RepeatableRead, func(tx pgx.Tx) error {
		var err error
		progress, err = h.read(ctx, tx, params)
		return err
	})
	if err != nil {
		return salesassessment.Input{}, "", err
	}
	zone, err := time.LoadLocation(progress.Timezone)
	if err != nil {
		return salesassessment.Input{}, "", err
	}
	words := reporting.ExplanationCopy(progress.Evaluation.Context)
	in := SummaryOf(progress, zone, words)
	salesassessment.DisplayFigures(&in, string(words.Lang()))
	return in, progress.Evaluation.EvaluationKey, nil
}

// SummaryOf turns a Progress reading into the summary's cells. A figure the
// page withholds or has not started is left out, so no sentence can rest on it.
func SummaryOf(progress crmcontracts.SalesProgress, zone *time.Location, words langcopy.Spoken) salesassessment.Input {
	period := summaryWindow(progress.Horizon, zone)
	figures := summaryFigures{currency: progress.Currency, period: period, actual: period}
	if progress.ActualWindow != nil {
		figures.actual = summaryWindow(*progress.ActualWindow, zone)
	}
	in := salesassessment.Input{View: salesassessment.ViewProgress, Frame: salesassessment.FrameIn{
		Timezone: progress.Timezone, Period: string(progress.Period.Kind), Window: period,
		Currency: progress.Currency, Today: progress.Cutoff.In(zone).Format(time.DateOnly),
	}}
	if progress.Scope.Label != nil {
		in.Frame.Scope = *progress.Scope.Label
	}
	figures.goal("progress:goal", words.Say(summaryWords.Goal), progress.Goal, words)
	figures.amount("progress:closed", words.Say(summaryWords.Closed), progress.Closed, figures.actual)
	figures.comparison(progress, words)
	figures.amount("progress:planned", words.Say(summaryWords.Planned), progress.Planned, period)
	if progress.Closed.AmountMinor != nil && *progress.Closed.AmountMinor > 0 {
		split := progress.Categories
		figures.part("progress:product", words.Say(summaryWords.Product), split.ProductMinor, progress.Closed.Status)
		figures.part("progress:service", words.Say(summaryWords.Service), split.ServiceMinor, progress.Closed.Status)
		figures.part("progress:unresolved", words.Say(summaryWords.Unresolved), split.UnresolvedMinor, progress.Closed.Status)
	}
	for index, member := range progress.Members[:min(len(progress.Members), summaryMembers)] {
		id := "member:" + strconv.Itoa(index+1)
		figures.goal(id+":goal", words.Say(summaryWords.Goal)+" · "+member.Name, member.Goal, words)
		figures.amount(id+":closed", words.Say(summaryWords.Closed)+" · "+member.Name, member.Closed, figures.actual)
	}
	in.Cells = figures.cells
	return in
}

// summaryFigures collects the cells of one reading in page order.
type summaryFigures struct {
	currency, period, actual string
	cells                    []salesassessment.CellIn
}

func (f *summaryFigures) money(id, label string, minor int64, window, coverage string) *salesassessment.CellIn {
	f.cells = append(f.cells, salesassessment.CellIn{
		ID: id, Label: label, Unit: f.currency, Window: window, Coverage: coverage,
		Value: salesassessment.FigureText(float64(minor), f.currency),
	})
	return &f.cells[len(f.cells)-1]
}

func (f *summaryFigures) amount(id, label string, amount crmcontracts.SalesProgressAmount, window string) {
	if amount.AmountMinor == nil {
		return
	}
	switch amount.Status {
	case crmcontracts.SalesProgressAmountStatusOk:
		f.money(id, label, *amount.AmountMinor, window, string(crmcontracts.ReportingStatusOk))
	case crmcontracts.SalesProgressAmountStatusPartial:
		f.money(id, label, *amount.AmountMinor, window, string(crmcontracts.ReportingStatusPartial))
	}
}

// part adds one category of closed revenue under the closed figure's own
// coverage. A category with nothing in it is not a figure to read.
func (f *summaryFigures) part(id, label string, minor int64, closed crmcontracts.SalesProgressAmountStatus) {
	if minor > 0 {
		f.amount(id, label, crmcontracts.SalesProgressAmount{Status: closed, AmountMinor: &minor}, f.actual)
	}
}

// goal adds a goal set for the whole period, or the months that have one as
// a partial figure. Any other goal is no figure.
func (f *summaryFigures) goal(id, label string, goal crmcontracts.SalesProgressGoal, words langcopy.Spoken) {
	if goal.AmountMinor == nil {
		return
	}
	switch goal.Status {
	case crmcontracts.SalesProgressGoalStatusSet, crmcontracts.SalesProgressGoalStatusZero:
		f.money(id, label, *goal.AmountMinor, f.period, string(crmcontracts.ReportingStatusOk))
	case crmcontracts.SalesProgressGoalStatusIncomplete:
		cell := f.money(id, label, *goal.AmountMinor, f.period, string(crmcontracts.ReportingStatusPartial))
		cell.CoverageDetail = strconv.Itoa(goal.MonthsSet) + "/" + strconv.Itoa(goal.MonthsNeeded) + " " + words.Say(summaryWords.GoalMonths)
	}
}

// comparison adds what the page compares: the amount missing to the goal or
// above it. The share reached is the whole percent the page prints.
func (f *summaryFigures) comparison(progress crmcontracts.SalesProgress, words langcopy.Spoken) {
	if delta := progress.DeltaMinor; delta != nil && *delta < 0 {
		f.money("progress:missing", words.Say(summaryWords.Missing), -*delta, f.period, string(crmcontracts.ReportingStatusOk))
	} else if delta != nil && *delta > 0 {
		f.money("progress:over", words.Say(summaryWords.Over), *delta, f.period, string(crmcontracts.ReportingStatusOk))
	}
	if progress.Completion != nil {
		f.cells = append(f.cells, salesassessment.CellIn{
			ID: "progress:completion", Label: words.Say(summaryWords.Completion), Unit: "percent", Window: f.actual,
			Coverage: string(crmcontracts.ReportingStatusOk), Value: salesassessment.FigureText(math.Round(*progress.Completion*100), "percent"),
		})
	}
}

func summaryWindow(window crmcontracts.ReportingWindow, zone *time.Location) string {
	return window.StartAt.In(zone).Format(time.DateOnly) + " to " + window.EndAt.In(zone).Format(time.DateOnly)
}
