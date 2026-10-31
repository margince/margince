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
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
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

// fewestColleagues is how many live colleagues besides the reader must share a
// total that any colleague contributes to, while the switch is off. With one,
// the total minus the reader's own figure is that colleague's figure.
const fewestColleagues = 2

// requireNoLoneColleague refuses, while the switch is off, an answer with a
// group that other seats contribute to but fewer than fewestColleagues live
// colleagues do. Each group is the owners whose rows make up one figure. A
// departed owner's rows still count as a colleague's, and do not count toward
// the floor.
func requireNoLoneColleague(ctx context.Context, tx pgx.Tx, groups [][]ids.UUID) error {
	allowed, err := individualPerformance(ctx, tx)
	if err != nil || allowed {
		return err
	}
	thin, err := thinGroups(ctx, tx, groups)
	if err != nil {
		return err
	}
	if slices.Contains(thin, true) {
		return auth.IndividualPerformanceOffError{}
	}
	return nil
}

// thinGroups answers, per group, whether other seats are in it but fewer than
// fewestColleagues live colleagues besides the reader are.
func thinGroups(ctx context.Context, tx pgx.Tx, groups [][]ids.UUID) ([]bool, error) {
	p, ok := principal.Actor(ctx)
	if !ok {
		return nil, errors.New("compose: no actor bound to context")
	}
	others := map[ids.UUID]bool{}
	for _, group := range groups {
		for _, owner := range group {
			if !owner.IsZero() && owner != p.UserID {
				others[owner] = true
			}
		}
	}
	out := make([]bool, len(groups))
	if len(others) == 0 {
		return out, nil
	}
	live, err := liveColleagues(ctx, tx, slices.Collect(maps.Keys(others)))
	if err != nil {
		return nil, err
	}
	for index, group := range groups {
		contributes, counted := false, 0
		for _, owner := range group {
			if others[owner] {
				contributes = true
				if live[owner] {
					counted++
				}
			}
		}
		out[index] = contributes && counted < fewestColleagues
	}
	return out, nil
}

// targetFloor answers, while the switch is off, which team and company
// targets keep enough colleagues behind them to show. A target is in effect a
// colleague's own when its scope has other members but fewer than
// fewestColleagues live ones besides the reader. Archived teams are checked
// too, and a team the check never reads is not on the list, so it is withheld.
func targetFloor(ctx context.Context, tx pgx.Tx) (reporting.TargetFloor, error) {
	allowed, err := individualPerformance(ctx, tx)
	if err != nil || allowed {
		return reporting.TargetFloor{}, err
	}
	rows, err := tx.Query(ctx, `SELECT t.id, array_remove(array_agg(tm.user_id), NULL) FROM team t
		LEFT JOIN team_membership tm ON tm.team_id = t.id
		GROUP BY t.id ORDER BY t.id`)
	if err != nil {
		return reporting.TargetFloor{}, fmt.Errorf("compose: reading team members for the target floor: %w", err)
	}
	var teams []ids.UUID
	var members [][]ids.UUID
	for rows.Next() {
		var team ids.UUID
		var seats []ids.UUID
		if err := rows.Scan(&team, &seats); err != nil {
			rows.Close()
			return reporting.TargetFloor{}, fmt.Errorf("compose: reading team members for the target floor: %w", err)
		}
		teams, members = append(teams, team), append(members, seats)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return reporting.TargetFloor{}, fmt.Errorf("compose: reading team members for the target floor: %w", err)
	}
	var everyone []ids.UUID
	if err := tx.QueryRow(ctx, `SELECT COALESCE(array_agg(u.id), '{}') FROM app_user u WHERE NOT u.is_agent`).Scan(&everyone); err != nil {
		return reporting.TargetFloor{}, fmt.Errorf("compose: reading the members for the company target floor: %w", err)
	}
	thin, err := thinGroups(ctx, tx, append(members, everyone))
	if err != nil {
		return reporting.TargetFloor{}, err
	}
	floor := reporting.TargetFloor{Applies: true, Workspace: !thin[len(teams)], Teams: []ids.UUID{}}
	for index, team := range teams {
		if !thin[index] {
			floor.Teams = append(floor.Teams, team)
		}
	}
	return floor, nil
}

// liveColleagues answers which of these seats are live human members.
func liveColleagues(ctx context.Context, tx pgx.Tx, seats []ids.UUID) (map[ids.UUID]bool, error) {
	rows, err := tx.Query(ctx, `SELECT u.id FROM app_user u WHERE u.id = ANY($1) AND NOT u.is_agent AND `+
		identity.LiveMemberSQL("u"), seats)
	if err != nil {
		return nil, fmt.Errorf("compose: reading which contributors are live members: %w", err)
	}
	found, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return nil, fmt.Errorf("compose: reading which contributors are live members: %w", err)
	}
	out := make(map[ids.UUID]bool, len(found))
	for _, id := range found {
		out[id] = true
	}
	return out, nil
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
// is that colleague's number, however old. It also clears every colleague's
// seat off the rows, which the export would otherwise print.
func requireFrozenShareMeasurable(ctx context.Context, tx pgx.Tx, scope forecasting.Scope, rows []SharedContribution) error {
	allowed, err := individualPerformance(ctx, tx)
	if err != nil || allowed {
		return err
	}
	if _, err := ResolveAnalyticsScope(ctx, tx, requestedFromForecastScope(scope)); err != nil {
		return err
	}
	owners, categories := []ids.UUID{}, []string{}
	for _, row := range rows {
		if row.Owner != nil {
			owners, categories = append(owners, *row.Owner), append(categories, row.Category)
		}
	}
	if err := requireNoLoneColleague(ctx, tx, categoryGroups(owners, categories)); err != nil {
		return err
	}
	for index := range rows {
		if rows[index].Owner != nil && !auth.MayMeasureSeat(ctx, false, *rows[index].Owner) {
			rows[index].Owner = nil
		}
	}
	return nil
}

// ForecastReaderContributions narrows the forecast's per-deal rows to what this
// reader may see one by one. While the switch is off a row names a seat. So
// only the reader's own deals stay, with no owner on them, as evidence does.
// The HTTP door and the forecast tool both ask it.
func ForecastReaderContributions(ctx context.Context, tx pgx.Tx, rows []forecasting.Contribution) ([]forecasting.Contribution, error) {
	allowed, err := individualPerformance(ctx, tx)
	if err != nil || allowed {
		return rows, err
	}
	out := make([]forecasting.Contribution, 0, len(rows))
	for _, row := range rows {
		owner, err := ids.Parse(row.Owner)
		if err != nil || !auth.MayMeasureSeat(ctx, false, owner) {
			continue
		}
		row.Owner = ""
		out = append(out, row)
	}
	return out, nil
}
