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
	"github.com/margince/margince/backend/internal/modules/deals"
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
	// Deals lists the open work, under the deal list's own row scope and masks.
	Deals *deals.Store
}

// GetSalesProgress answers closed revenue against the goal for one period.
func (h Reading) GetSalesProgress(w http.ResponseWriter, r *http.Request, params crmcontracts.GetSalesProgressParams) {
	ctx := r.Context()
	var out crmcontracts.SalesProgress
	err := h.DB.TxIsolated(ctx, pgx.RepeatableRead, func(tx pgx.Tx) error {
		var err error
		out, err = h.read(ctx, tx, params)
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
	if period.year < 2000 || period.year > 2200 {
		return progressFrame{}, httperr.Validation("year", "out_of_range", "choose a year from 2000 to 2200")
	}
	if period.month != nil && (*period.month < 1 || *period.month > 12) {
		return progressFrame{}, httperr.Validation("month", "out_of_range", "choose a month from 1 to 12")
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
// the request names none.
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

// readableScope is the population a reading or its deal list asks about. A
// named scope must be inside the reader's own lens. Analytics offers every
// seat a team's totals, which in a small team is a colleague's figure.
func (h Reading) readableScope(
	ctx context.Context, tx pgx.Tx, kind *crmcontracts.GetSalesProgressParamsScopeKind, id *openapi_types.UUID,
) (crmcontracts.ReportingScope, error) {
	scope, err := progressScope(ctx, kind, id)
	if err != nil || kind == nil {
		return scope, err
	}
	return h.Reports.GoalScope(ctx, tx, scope)
}

func (h Reading) read(ctx context.Context, tx pgx.Tx, params crmcontracts.GetSalesProgressParams) (crmcontracts.SalesProgress, error) {
	frame, err := h.frame(ctx, tx, progressPeriod{kind: params.Period, year: params.Year, month: params.Month})
	if err != nil {
		return crmcontracts.SalesProgress{}, err
	}
	scope, err := h.readableScope(ctx, tx, params.ScopeKind, params.ScopeId)
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
	sellers, err := h.Reports.GoalSellers(ctx, tx, headline.Result.Context.Scope)
	if err != nil {
		return crmcontracts.SalesProgress{}, err
	}
	goals, err := h.Reports.ReadOwnerGoals(ctx, tx, sellers, frame.period.year, frame.currency)
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
		Scope: scope, Period: crmcontracts.ReportingSelectionPeriodCustom, Interval: &horizon, TargetBasis: crmcontracts.ReportingSelectionTargetBasisMonth,
		CloseWindow: crmcontracts.ReportingSelectionCloseWindowCustom, CloseInterval: &horizon,
		Metrics: []crmcontracts.ReportingMetricID{crmcontracts.ReportingMetricIDBookingsWon, crmcontracts.ReportingMetricIDBookingsMix, crmcontracts.ReportingMetricIDOpenPipeline},
	}
	if !frame.started() {
		selection.Period, selection.Interval = crmcontracts.ReportingSelectionPeriodThisMonth, nil
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
		Scope: scope, Period: crmcontracts.ReportingSelectionPeriodCustom, Interval: &window, TargetBasis: crmcontracts.ReportingSelectionTargetBasisMonth, CloseWindow: crmcontracts.ReportingSelectionCloseWindowAllOpen,
		Metrics: []crmcontracts.ReportingMetricID{crmcontracts.ReportingMetricIDBookingsWon},
	})
}

func progressShell(frame progressFrame, headline reporting.Evaluation) crmcontracts.SalesProgress {
	out := crmcontracts.SalesProgress{
		Scope: headline.Result.Context.Scope, Horizon: frame.horizon, Cutoff: frame.now,
		Timezone: frame.zone.String(), Currency: frame.currency, State: crmcontracts.SalesProgressStateCurrent,
		Months: []crmcontracts.SalesProgressMonth{}, Members: []crmcontracts.SalesProgressMember{},
		PlannedValuedAt: headline.Result.Context.EvaluatedAt,
	}
	out.Period.Kind, out.Period.Year, out.Period.Month = crmcontracts.SalesProgressPeriodKind(frame.period.kind), frame.period.year, frame.period.month
	switch {
	case !frame.started():
		out.State = crmcontracts.SalesProgressStateNotStarted
	case !frame.now.Before(frame.horizon.EndAt):
		out.State = crmcontracts.SalesProgressStateEnded
	}
	if frame.started() {
		actual := headline.Result.Context.Interval
		out.ActualWindow = &actual
	}
	result := headline.Result
	out.Evaluation = &result
	return out
}
