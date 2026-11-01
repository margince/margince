// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// A manager's call, compared with the reading it was made against.
//
// The call keeps a baseline: the reading of its population at the moment it
// was made, taken the way a daily capture takes one. The input check reads
// the same population the same way today, and the forecast engine says what
// moved between the two. Both sides are read as the system, over every deal
// of the scope, so what a reader may see never shows up as movement.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/assurance"
	"github.com/margince/margince/backend/internal/modules/forecasting"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/shared/kernel/elapsed"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// forecastCallBaselineActor is who reads a call's population.
const forecastCallBaselineActor = "system:forecast-call-baseline"

// ForecastCallBaseline reads a call's population as the daily capture does.
func ForecastCallBaseline(
	ctx context.Context, tx pgx.Tx, scope forecasting.Scope, period forecasting.Period, at time.Time,
) (forecasting.Baseline, error) {
	currency, err := identity.BaseCurrencyOf(ctx, tx)
	if err != nil {
		return forecasting.Baseline{}, err
	}
	reader := principal.SystemActing(ctx, forecastCallBaselineActor)
	population := crmcontracts.ReportingScope{
		Kind: crmcontracts.ReportingScopeKind(scope.Kind), Id: (*openapi_types.UUID)(scope.ID),
	}
	return captureReadings(reader, tx, population, nil, period, at, currency)
}

// standingCallsSQL reads the calls no later call replaced, for periods that
// have not ended. It has no limit: the pass clears the findings of calls it
// did not raise again, so a call it skipped would lose its finding.
const standingCallsSQL = `
	SELECT c.id, c.period_start, c.period_end, c.scope_kind, c.scope_id,
	       c.amount_minor, c.currency, c.author_id, c.created_at
	  FROM forecast_call c
	 WHERE c.period_end >= $1
	   AND NOT EXISTS (SELECT 1 FROM forecast_call later WHERE later.supersedes_id = c.id)
	 ORDER BY c.created_at, c.id`

// assuranceCallSubjects compares each standing call with its baseline. A call
// with no baseline was made before calls kept one, and is left out. Its
// history is unavailable, and today's records cannot stand in for it.
func assuranceCallSubjects(pool *pgxpool.Pool) assurance.CallSubjectsFunc {
	store := newForecastStoreFor(pool)
	return func(ctx context.Context, tx pgx.Tx, now time.Time) ([]assurance.CallSubject, error) {
		zone, err := installationZoneTx(ctx, tx)
		if err != nil {
			return nil, err
		}
		rows, err := tx.Query(ctx, standingCallsSQL, now.In(zone).Format(time.DateOnly))
		if err != nil {
			return nil, fmt.Errorf("compose: reading the standing calls: %w", err)
		}
		calls, err := pgx.CollectRows(rows, scanStandingCall)
		if err != nil {
			return nil, fmt.Errorf("compose: reading the standing calls: %w", err)
		}
		out := make([]assurance.CallSubject, 0, len(calls))
		for _, call := range calls {
			subject, compared, err := compareCall(ctx, tx, store, call, now)
			if err != nil {
				return nil, err
			}
			if compared {
				out = append(out, subject)
			}
		}
		return out, nil
	}
}

func scanStandingCall(row pgx.CollectableRow) (forecasting.Call, error) {
	var call forecasting.Call
	err := row.Scan(&call.ID, &call.PeriodStart, &call.PeriodEnd, &call.Scope.Kind, &call.Scope.ID,
		&call.AmountMinor, &call.Currency, &call.AuthorID, &call.CreatedAt)
	return call, err
}

// compareCall answers one call's subject. compared is false for a call with
// no baseline.
func compareCall(
	ctx context.Context, tx pgx.Tx, store *forecasting.Store, call forecasting.Call, now time.Time,
) (assurance.CallSubject, bool, error) {
	baseline, found, err := store.CallBaselineTx(ctx, tx, call.ID)
	if err != nil || !found {
		return assurance.CallSubject{}, false, err
	}
	subject := assurance.CallSubject{
		CallID: call.ID.String(), Owner: call.AuthorID.String(),
		PeriodStart: call.PeriodStart, PeriodEnd: call.PeriodEnd, ScopeKind: call.Scope.Kind,
		CalledMinor: call.AmountMinor, Currency: call.Currency, SubmittedAt: baseline.TakenAt,
		BaselineSnapshotID: baseline.ID.String(),
	}
	if call.Scope.ID != nil {
		subject.ScopeID = call.Scope.ID.String()
	}
	period, same, err := callWindow(ctx, tx, call)
	if err != nil {
		return assurance.CallSubject{}, false, err
	}
	if !same {
		subject.Limitation = string(forecasting.LimitationPeriod)
		return subject, true, nil
	}
	today, err := ForecastCallBaseline(ctx, tx, call.Scope, period, now)
	if err != nil {
		return assurance.CallSubject{}, false, err
	}
	drift := forecasting.CompareWithBaseline(baseline,
		forecasting.TodaySide(today, now))
	subject.Limitation = string(drift.Limitation)
	subject.QualificationChanges = drift.QualificationChanges
	subject.Net = map[string]int64{}
	for reading, net := range drift.Net {
		subject.Net[string(reading)] = net
	}
	for _, deal := range drift.Deals {
		subject.DealIDs = append(subject.DealIDs, deal.DealID)
	}
	return subject, true, nil
}

// callWindow resolves the call's window afresh from the call's own period,
// asked at its middle day so no zone moves it across a boundary. same is
// false when the calendar no longer has it, such as after the financial year
// was moved. No reading can then be taken over the call's period.
func callWindow(ctx context.Context, tx pgx.Tx, call forecasting.Call) (forecasting.Period, bool, error) {
	zone, err := installationZoneTx(ctx, tx)
	if err != nil {
		return forecasting.Period{}, false, err
	}
	days := elapsed.Days(call.PeriodStart, call.PeriodEnd, zone)
	middle := call.PeriodStart.AddDate(0, 0, days/2)
	for _, kind := range []forecasting.PeriodKind{forecasting.PeriodQuarter, forecasting.PeriodMonth, forecasting.PeriodWeek} {
		period, _, err := ForecastPeriodAt(ctx, tx, kind, middle)
		if err != nil {
			return forecasting.Period{}, false, err
		}
		if sameDate(period.StartDate, call.PeriodStart) && sameDate(period.EndDate, call.PeriodEnd) {
			return period, true, nil
		}
	}
	return forecasting.Period{}, false, nil
}

func sameDate(a, b time.Time) bool { return a.Format(time.DateOnly) == b.Format(time.DateOnly) }

// assuranceDealSubjects reads one deal for a pass queued because it changed.
func assuranceDealSubjects(
	ctx context.Context, tx pgx.Tx, asOf time.Time, coverage []assurance.SourceCoverage, deal ids.UUID,
) ([]assurance.Subject, error) {
	return assuranceSubjects(ctx, tx, asOf, coverage, []ids.UUID{deal})
}

// assuranceScanner is the input check as every caller runs it.
func assuranceScanner(pool *pgxpool.Pool, store *assurance.Store) *assurance.Scanner {
	return assurance.NewScanner(store, AssuranceSubjects, AssuranceCoverage, AssuranceConfig()).
		WithSignalSubjects(AssuranceSignalSubjects).
		WithCallSubjects(assuranceCallSubjects(pool)).
		WithDealSubjects(assuranceDealSubjects)
}
