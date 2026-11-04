// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The installation's "Show individual performance" switch, applied where the
// analytics lens decides whose work a reader measures.
//
// Off, a reader still measures themselves and any team or company total. They
// lose every answer keyed to one colleague. They also lose every total small
// enough to be one: a team of two minus their own figure is the colleague's.

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/forecasting"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/reporting"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// individualPerformance answers the switch for this request: the value its door
// bound, or one read in the caller's transaction.
func individualPerformance(ctx context.Context, tx pgx.Tx) (bool, error) {
	if allowed, bound := auth.IndividualPerformanceOf(ctx); bound {
		return allowed, nil
	}
	return identity.IndividualPerformanceTx(ctx, tx)
}

// bindIndividualPerformance reads the switch once for a job run, so every
// member the run acts as answers to the same value.
func bindIndividualPerformance(ctx context.Context, pool *pgxpool.Pool) (context.Context, bool, error) {
	allowed, err := readIndividualPerformance(ctx, pool)
	if err != nil {
		return ctx, false, err
	}
	return auth.WithIndividualPerformance(ctx, allowed), allowed, nil
}

// readIndividualPerformance reads the switch now, ignoring any bound value, for
// a step that must not act on an answer read earlier in its run.
func readIndividualPerformance(ctx context.Context, pool *pgxpool.Pool) (bool, error) {
	var allowed bool
	err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		var err error
		allowed, err = identity.IndividualPerformanceTx(ctx, tx)
		return err
	})
	if err != nil {
		return false, fmt.Errorf("compose: reading whether individual performance is shown: %w", err)
	}
	return allowed, nil
}

// requireMeasurableSeat refuses naming anyone but the reader while the
// switch is off.
func requireMeasurableSeat(ctx context.Context, tx pgx.Tx, seat ids.UUID) error {
	allowed, err := individualPerformance(ctx, tx)
	if err != nil {
		return err
	}
	if auth.MayMeasureSeat(ctx, allowed, seat) {
		return nil
	}
	return auth.IndividualPerformanceOffError{}
}

// requireNoLoneColleagueIn runs requireNoLoneColleague over a statement that
// answers one array of contributing owners per group, the way contributorsSQL
// renders it. Nothing is read while the switch is on.
func requireNoLoneColleagueIn(ctx context.Context, tx pgx.Tx, frame reportFrame, sql string, args []any) error {
	allowed, err := individualPerformance(ctx, tx)
	if err != nil || allowed || sql == "" {
		return err
	}
	bound, boundArgs, err := bindReportTokens(ctx, tx, frame, sql, args)
	if err != nil {
		return err
	}
	rows, err := tx.Query(ctx, bound, boundArgs...)
	if err != nil {
		return fmt.Errorf("compose: reading who contributes to an answer: %w", err)
	}
	groups, err := pgx.CollectRows(rows, pgx.RowTo[[]ids.UUID])
	if err != nil {
		return fmt.Errorf("compose: reading who contributes to an answer: %w", err)
	}
	return requireNoLoneColleague(ctx, tx, groups)
}

// contributorsSQL renders the statement requireNoLoneColleagueIn reads: the
// distinct owners behind each group of an answer, over its own `FROM` and `WHERE`.
// "" when the spec names no seat to count.
func contributorsSQL(spec reportSpec, where, groupExprs []string) string {
	owner := contributorColumn(spec)
	if owner == "" {
		return ""
	}
	sql := fmt.Sprintf("SELECT array_agg(DISTINCT %s) FILTER (WHERE %s IS NOT NULL) FROM %s", owner, owner, spec.fromClause())
	if len(where) > 0 {
		sql += " WHERE " + strings.Join(where, " AND ")
	}
	if len(groupExprs) > 0 {
		sql += " GROUP BY " + strings.Join(groupExprs, ", ")
	}
	return sql
}

// dimensionExprs is the SQL each named dimension groups on.
func dimensionExprs(spec reportSpec, names []string) []string {
	out := make([]string, 0, len(names))
	for _, name := range names {
		out = append(out, spec.dimensions[name])
	}
	return out
}

// contributorColumn is the seat a spec's rows are credited to: the owner a
// population narrows on, else the first seat column it names.
func contributorColumn(spec reportSpec) string {
	if measuresOwners(spec) {
		return colOwnerID
	}
	for _, expr := range slices.Sorted(maps.Keys(seatColumns)) {
		if slices.Contains(slices.Collect(maps.Values(spec.dimensions)), expr) ||
			slices.Contains(slices.Collect(maps.Values(spec.filters)), expr) {
			return expr
		}
	}
	return ""
}

// withholdColleagues applies the switch to one metric's facts. Every
// colleague's fact is marked withheld from owner charts. A figure that one
// colleague besides the reader makes up loses its facts and says why.
func withholdColleagues(
	ctx context.Context, tx pgx.Tx, frame crmcontracts.ReportingContext, owners reporting.MeasurableOwners,
	facts []reporting.Fact, coverage crmcontracts.ReportingCoverage,
) ([]reporting.Fact, crmcontracts.ReportingCoverage, error) {
	markOwnerWithheld(owners, facts)
	if !owners.IndividualOff {
		return facts, coverage, nil
	}
	zone, err := time.LoadLocation(frame.Timezone)
	if err != nil {
		return nil, coverage, fmt.Errorf("compose: the reporting zone %q is not a zone: %w", frame.Timezone, err)
	}
	err = requireNoLoneColleague(ctx, tx, factGroups(facts, zone))
	if !errors.As(err, new(auth.IndividualPerformanceOffError)) {
		return facts, coverage, err
	}
	return []reporting.Fact{}, loneColleagueCoverage(frame), nil
}

// loneColleagueCoverage is a figure withheld because one colleague besides the
// reader makes it up.
func loneColleagueCoverage(frame crmcontracts.ReportingContext) crmcontracts.ReportingCoverage {
	withheld := reportingGap(reportingUnavailable, reporting.ExplanationCopy(frame).Say(reportingCoverageWords.LoneColleague))
	withheld.Withheld = true
	return withheld
}

// thinTeamCoverage withholds a figure of a population that populationBelowFloor holds back.
func thinTeamCoverage(frame crmcontracts.ReportingContext) crmcontracts.ReportingCoverage {
	withheld := reportingGap(reportingUnavailable, reporting.ExplanationCopy(frame).Say(reportingCoverageWords.ThinTeam))
	withheld.Withheld = true
	return withheld
}

// withoutLoneColleagueMetrics drops, from a frozen edition's facts, every
// metric whose figure a lone colleague besides the reader makes up, and says
// whether it dropped any.
func withoutLoneColleagueMetrics(ctx context.Context, tx pgx.Tx, facts []reporting.Fact) ([]reporting.Fact, bool, error) {
	byMetric := map[crmcontracts.ReportingMetricID][]reporting.Fact{}
	for _, fact := range facts {
		byMetric[fact.Metric] = append(byMetric[fact.Metric], fact)
	}
	lone := map[crmcontracts.ReportingMetricID]bool{}
	for metric, metricFacts := range byMetric {
		err := requireNoLoneColleague(ctx, tx, factGroups(metricFacts, time.UTC))
		if errors.As(err, new(auth.IndividualPerformanceOffError)) {
			lone[metric] = true
		} else if err != nil {
			return nil, false, err
		}
	}
	if len(lone) == 0 {
		return facts, false, nil
	}
	kept := make([]reporting.Fact, 0, len(facts))
	for _, fact := range facts {
		if !lone[fact.Metric] {
			kept = append(kept, fact)
		}
	}
	return kept, true, nil
}

// factGroups lists the owners behind the figures a metric's facts are drawn
// into: its total per reading, and each stage, breakdown group and week.
func factGroups(facts []reporting.Fact, zone *time.Location) [][]ids.UUID {
	byKey := map[string][]ids.UUID{}
	for _, fact := range facts {
		keys := []string{"total:" + fact.ContextID, "stage:" + fact.ContextID + ":" + fact.StageID, "group:" + fact.ContextID + ":" + fact.GroupKey}
		if at := fact.At(); at != nil {
			year, week := at.In(zone).ISOWeek()
			keys = append(keys, fmt.Sprintf("week:%s:%d-%d", fact.ContextID, year, week))
		}
		for _, key := range keys {
			byKey[key] = append(byKey[key], fact.OwnerID)
		}
	}
	out := make([][]ids.UUID, 0, len(byKey))
	for _, key := range slices.Sorted(maps.Keys(byKey)) {
		out = append(out, byKey[key])
	}
	return out
}

// forecastDealGroups lists the owners behind the forecast figures of a set of deals.
// An unowned deal names no seat.
func forecastDealGroups(deals []forecasting.Deal) [][]ids.UUID {
	owners, categories := []ids.UUID{}, []string{}
	for _, deal := range deals {
		if owner, err := ids.Parse(deal.Owner); err == nil {
			owners, categories = append(owners, owner), append(categories, deal.Category)
		}
	}
	return categoryGroups(owners, categories)
}

// categoryGroups is a forecast's figures by who makes them up: the whole set,
// and each forecast category. owners and categories are parallel.
func categoryGroups(owners []ids.UUID, categories []string) [][]ids.UUID {
	byCategory := map[string][]ids.UUID{}
	for index, owner := range owners {
		byCategory[categories[index]] = append(byCategory[categories[index]], owner)
	}
	out := [][]ids.UUID{owners}
	for _, category := range slices.Sorted(maps.Keys(byCategory)) {
		out = append(out, byCategory[category])
	}
	return out
}

// explainWithheldOwners says on each owner chart why colleagues are missing.
// Without it an empty chart reads as a team that sold nothing.
func explainWithheldOwners(ctx context.Context, tx pgx.Tx, out *reporting.Evaluation) error {
	allowed, err := individualPerformance(ctx, tx)
	if err != nil || allowed {
		return err
	}
	reason := reporting.ExplanationCopy(out.Result.Context).Say(reportingCoverageWords.IndividualOff)
	for index := range out.Result.Charts {
		if chart := &out.Result.Charts[index]; chart.Kind == reportingOwnerAttainment {
			chart.Coverage.Withheld = true
			chart.Coverage.Reason = &reason
		}
	}
	return nil
}

// requireFrozenShareMeasurable asks a frozen reading what a live one is asked
// on every read, while the switch is off. A snapshot of a colleague's forecast
// is that colleague's number, however old. It answers only the reader's own
// rows, which the export prints, as the live per-deal rows do. limited says at
// least one row was withheld, as ForecastReaderContributions says it.
func requireFrozenShareMeasurable(ctx context.Context, tx pgx.Tx, scope forecasting.Scope, rows []SharedContribution) (visible []SharedContribution, limited bool, err error) {
	allowed, err := individualPerformance(ctx, tx)
	if err != nil || allowed {
		return rows, false, err
	}
	resolved, err := ResolveAnalyticsScope(ctx, tx, requestedFromForecastScope(scope))
	if err != nil {
		return nil, false, err
	}
	if err := requirePopulationFloor(ctx, tx, resolved); err != nil {
		return nil, false, err
	}
	owners, categories := []ids.UUID{}, []string{}
	for _, row := range rows {
		if row.Owner != nil {
			owners, categories = append(owners, *row.Owner), append(categories, row.Category)
		}
	}
	if err := requireNoLoneColleague(ctx, tx, categoryGroups(owners, categories)); err != nil {
		return nil, false, err
	}
	own := make([]SharedContribution, 0, len(rows))
	for _, row := range rows {
		if row.Owner != nil && readerOwnsRow(ctx, *row.Owner) {
			own = append(own, row)
		}
	}
	return own, len(own) < len(rows), nil
}

// readerOwnsRow is the per-deal row rule while the switch is off: a row stays
// only when the deal is the reader's own. Shares and live rows both ask it.
func readerOwnsRow(ctx context.Context, owner ids.UUID) bool {
	return auth.MayMeasureSeat(ctx, false, owner)
}

// ForecastReaderContributions narrows the forecast's per-deal rows to what this
// reader may see one by one. While the switch is off a row names a seat. So
// only the reader's own deals stay, with no owner on them, as evidence does.
// The HTTP door and the forecast tool both ask it. limited says at least one
// row was withheld, which both doors tell their reader.
func ForecastReaderContributions(ctx context.Context, tx pgx.Tx, rows []forecasting.Contribution) (visible []forecasting.Contribution, limited bool, err error) {
	allowed, err := individualPerformance(ctx, tx)
	if err != nil || allowed {
		return rows, false, err
	}
	out := make([]forecasting.Contribution, 0, len(rows))
	for _, row := range rows {
		owner, err := ids.Parse(row.Owner)
		if err != nil || !readerOwnsRow(ctx, owner) {
			continue
		}
		row.Owner = ""
		out = append(out, row)
	}
	return out, len(out) < len(rows), nil
}
