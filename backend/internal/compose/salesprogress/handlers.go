// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package salesprogress

import (
	"context"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/reporting"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// Reading serves My Progress and Team Progress. It reads the
// figures from the reporting evaluation Analytics uses and the goals from the
// sales targets, so both read one scope and window alike.
type Reading struct {
	DB      *database.DB
	Reports *reporting.Service
	// Members lists a scope's sellers, and Measurable the owners the reader
	// may measure one by one: the rules Analytics reads its population with.
	Members    func(ctx context.Context, tx pgx.Tx, scope crmcontracts.ReportingScope) ([]ids.UUID, error)
	Measurable func(ctx context.Context, tx pgx.Tx) (reporting.MeasurableOwners, error)
}

// GetSalesProgress answers closed revenue against the goal for one period.
func (h Reading) GetSalesProgress(w http.ResponseWriter, r *http.Request, params crmcontracts.GetSalesProgressParams) {
	var out crmcontracts.SalesProgress
	err := h.DB.TxIsolated(r.Context(), pgx.RepeatableRead, func(tx pgx.Tx) error {
		var err error
		out, err = h.read(r.Context(), tx, params)
		return err
	})
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

// progressPeriod is the month or calendar year a reading asks about.
type progressPeriod struct {
	kind  crmcontracts.GetSalesProgressParamsPeriod
	year  int
	month *int
}

func (p progressPeriod) yearly() bool { return p.kind == "year" }

// progressFrame is the period a reading is about, resolved in the reporting zone.
type progressFrame struct {
	period   progressPeriod
	zone     *time.Location
	now      time.Time
	currency string
	horizon  crmcontracts.ReportingWindow
	year     crmcontracts.ReportingWindow
}

func (f progressFrame) started() bool { return f.horizon.StartAt.Before(f.now) }

// months lists the months of the year the period covers, 1 to 12.
func (f progressFrame) months() []int {
	if !f.period.yearly() {
		return []int{*f.period.month}
	}
	return []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}
}

func (h Reading) frame(ctx context.Context, tx pgx.Tx, period progressPeriod) (progressFrame, error) {
	if !period.yearly() && period.month == nil {
		return progressFrame{}, httperr.Validation("month", "required", "choose the month to read")
	}
	calendar, err := h.Reports.Calendar(ctx, tx)
	if err != nil {
		return progressFrame{}, err
	}
	zone, err := time.LoadLocation(calendar.Timezone)
	if err != nil {
		return progressFrame{}, err
	}
	out := progressFrame{period: period, zone: zone, now: h.Reports.Now(), currency: calendar.Currency}
	yearStart := time.Date(period.year, time.January, 1, 0, 0, 0, 0, zone)
	out.year = crmcontracts.ReportingWindow{StartAt: yearStart, EndAt: yearStart.AddDate(1, 0, 0)}
	out.horizon = out.year
	if !period.yearly() {
		start := time.Date(period.year, time.Month(*period.month), 1, 0, 0, 0, 0, zone)
		out.horizon = crmcontracts.ReportingWindow{StartAt: start, EndAt: start.AddDate(0, 1, 0)}
	}
	return out, nil
}

// progressScope is the population asked about: the caller's own figures when
// the request names none. The evaluation checks the caller may read it.
func progressScope(ctx context.Context, kind *crmcontracts.GetSalesProgressParamsScopeKind, id *openapi_types.UUID) (crmcontracts.ReportingScope, error) {
	if kind != nil {
		if id == nil {
			return crmcontracts.ReportingScope{}, httperr.Validation("scope_id", "required", "choose the seller or team to read")
		}
		return crmcontracts.ReportingScope{Kind: crmcontracts.ReportingScopeKind(*kind), Id: id}, nil
	}
	actor, ok := principal.Actor(ctx)
	if !ok {
		return crmcontracts.ReportingScope{}, apperrors.ErrPermissionDenied
	}
	self := actor.UserID
	if actor.Type == principal.PrincipalAgent {
		self = actor.OnBehalfOf
	}
	if self.IsZero() {
		return crmcontracts.ReportingScope{}, apperrors.ErrPermissionDenied
	}
	own := openapi_types.UUID(self)
	return crmcontracts.ReportingScope{Kind: crmcontracts.ReportingScopeKindOwner, Id: &own}, nil
}

func (h Reading) read(ctx context.Context, tx pgx.Tx, params crmcontracts.GetSalesProgressParams) (crmcontracts.SalesProgress, error) {
	frame, err := h.frame(ctx, tx, progressPeriod{kind: params.Period, year: params.Year, month: params.Month})
	if err != nil {
		return crmcontracts.SalesProgress{}, err
	}
	scope, err := progressScope(ctx, params.ScopeKind, params.ScopeId)
	if err != nil {
		return crmcontracts.SalesProgress{}, err
	}
	headline, err := h.Reports.EvaluateFactsTx(ctx, tx, headlineSelection(scope, frame))
	if err != nil {
		return crmcontracts.SalesProgress{}, err
	}
	year := headline
	if !frame.period.yearly() {
		if year, err = h.yearEvaluation(ctx, tx, scope, frame); err != nil {
			return crmcontracts.SalesProgress{}, err
		}
	}
	sellers, err := h.sellers(ctx, tx, headline.Result.Context)
	if err != nil {
		return crmcontracts.SalesProgress{}, err
	}
	goals, err := h.goals(ctx, tx, frame, sellers)
	if err != nil {
		return crmcontracts.SalesProgress{}, err
	}
	out := progressShell(frame, headline)
	fillFigures(frame, headline, goals, sellers, &out)
	fillMonths(frame, year, goals, sellers, &out)
	if err := h.fillMembers(ctx, tx, frame, headline, goals, sellers, &out); err != nil {
		return out, err
	}
	if err := h.fillItems(ctx, tx, headline, &out); err != nil {
		return out, err
	}
	if scope.Kind == crmcontracts.ReportingScopeKindOwner {
		out.Capabilities.WriteGoals, err = h.Reports.CanSetGoals(ctx, tx, ids.UUID(*scope.Id))
	}
	return out, err
}

// headlineSelection reads closed revenue to the cutoff and the open deals
// expected in the whole period. A period that has not started reads the open
// deals alone, since it has no actuals yet.
func headlineSelection(scope crmcontracts.ReportingScope, frame progressFrame) crmcontracts.ReportingSelection {
	horizon := frame.horizon
	selection := crmcontracts.ReportingSelection{
		Scope: scope, Period: "custom", Interval: &horizon, TargetBasis: "month",
		CloseWindow: "custom", CloseInterval: &horizon,
		Metrics: []crmcontracts.ReportingMetricID{crmcontracts.ReportingMetricIDBookingsWon, crmcontracts.ReportingMetricIDBookingsMix, crmcontracts.ReportingMetricIDOpenPipeline},
	}
	if !frame.started() {
		selection.Period, selection.Interval = "this_month", nil
		selection.Metrics = []crmcontracts.ReportingMetricID{crmcontracts.ReportingMetricIDOpenPipeline}
	}
	return selection
}

// yearEvaluation reads the closed revenue of the year around a month, for the
// monthly table. A year that has not started has none.
func (h Reading) yearEvaluation(ctx context.Context, tx pgx.Tx, scope crmcontracts.ReportingScope, frame progressFrame) (reporting.Evaluation, error) {
	if !frame.year.StartAt.Before(frame.now) {
		return reporting.Evaluation{}, nil
	}
	window := frame.year
	return h.Reports.EvaluateFactsTx(ctx, tx, crmcontracts.ReportingSelection{
		Scope: scope, Period: "custom", Interval: &window, TargetBasis: "month", CloseWindow: "all_open",
		Metrics: []crmcontracts.ReportingMetricID{crmcontracts.ReportingMetricIDBookingsWon},
	})
}

func progressShell(frame progressFrame, headline reporting.Evaluation) crmcontracts.SalesProgress {
	out := crmcontracts.SalesProgress{
		Scope: headline.Result.Context.Scope, Horizon: frame.horizon, Cutoff: frame.now,
		Timezone: frame.zone.String(), Currency: frame.currency, State: "current",
		Months: []crmcontracts.SalesProgressMonth{}, Members: []crmcontracts.SalesProgressMember{},
		PlannedValuedAt: headline.Result.Context.EvaluatedAt,
	}
	out.Period.Kind, out.Period.Year, out.Period.Month = crmcontracts.SalesProgressPeriodKind(frame.period.kind), frame.period.year, frame.period.month
	switch {
	case !frame.started():
		out.State = "not_started"
	case !frame.now.Before(frame.horizon.EndAt):
		out.State = "ended"
	}
	if frame.started() {
		actual := headline.Result.Context.Interval
		out.ActualWindow = &actual
	}
	result := headline.Result
	out.Evaluation = &result
	return out
}
