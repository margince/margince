// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package reporting

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// windowMonths lists the calendar months a target window covers, by year. A
// window that is not whole months of the reporting zone covers none.
func windowMonths(window crmcontracts.ReportingWindow, zone *time.Location) (map[int][]int, bool) {
	start := window.StartAt.In(zone)
	if start.Day() != 1 || !start.Equal(time.Date(start.Year(), start.Month(), 1, 0, 0, 0, 0, zone)) {
		return nil, false
	}
	months := map[int][]int{}
	month := start
	for range goalMonths {
		if !month.Before(window.EndAt) {
			break
		}
		months[month.Year()] = append(months[month.Year()], int(month.Month()))
		month = month.AddDate(0, 1, 0)
	}
	return months, len(months) > 0 && month.Equal(window.EndAt)
}

// windowGoals are the published monthly goals over one target window.
type windowGoals struct {
	currency string
	months   map[int][]int
	years    map[int]OwnerGoals
}

// goalsOver reads the owners' goals for the months of window. It answers
// false for a window the monthly goals cannot fill.
func (s *Service) goalsOver(ctx context.Context, tx pgx.Tx, owners []ids.UUID, window crmcontracts.ReportingWindow) (windowGoals, bool, error) {
	calendar, err := s.calendar(ctx, tx)
	if err != nil {
		return windowGoals{}, false, err
	}
	zone, err := location(calendar)
	if err != nil {
		return windowGoals{}, false, err
	}
	out := windowGoals{currency: calendar.Currency, years: map[int]OwnerGoals{}}
	var whole bool
	if out.months, whole = windowMonths(window, zone); !whole || len(owners) == 0 {
		return out, false, nil
	}
	for year := range out.months {
		cells, err := yearTargets(ctx, tx, owners, year, false)
		if err != nil {
			return out, false, err
		}
		out.years[year] = OwnerGoals{Readable: true, Currency: calendar.Currency, Years: cells}
	}
	return out, true, nil
}

// over is the goal of owners across the window, when each has every month.
func (g windowGoals) over(owners []ids.UUID) (int64, bool) {
	var total int64
	for year, months := range g.months {
		goal := g.years[year].Over(owners, months, false)
		complete := goal.Status == crmcontracts.SalesProgressGoalStatusSet || goal.Status == crmcontracts.SalesProgressGoalStatusZero
		if !complete || total > values.MaxExactInteger-*goal.AmountMinor {
			return 0, false
		}
		total += *goal.AmountMinor
	}
	return total, true
}

// fromGoals reports a held target summed from sales goals. Such a target is
// no stored row, so it carries no id and the revision no stored target has.
func fromGoals(target crmcontracts.ReportingTarget) bool { return target.Revision == 0 }

// targetSource names a held target as a sales goal when it is one. That is a
// sum of goals, or the stored month of one seller, which Progress reads too.
func targetSource(target crmcontracts.ReportingTarget) crmcontracts.ReportingMetricTargetSource {
	if _, _, monthly := planOwnedIdentity(target.Definition); monthly || fromGoals(target) {
		return crmcontracts.ReportingMetricTargetSourceSalesGoals
	}
	return crmcontracts.ReportingMetricTargetSourceTarget
}

func goalTarget(frame crmcontracts.ReportingContext, scope crmcontracts.ReportingScope, value int64) crmcontracts.ReportingTarget {
	return crmcontracts.ReportingTarget{
		Definition: crmcontracts.ReportingTargetInput{Metric: goalMetric, Scope: scope, Value: value},
		Interval:   *frame.TargetInterval, Unit: frame.Currency,
	}
}

// goalFigure is one figure the goals may fill: its scope and the sellers
// whose goals add up to it.
type goalFigure struct {
	scope   crmcontracts.ReportingScope
	sellers []ids.UUID
}

// holdGoals stands sales won against the published sales goals where no target
// row matched. An owner stands against their own months, any other scope
// against the sum of its sellers. Goals span every pipeline, so an evaluation
// of one pipeline holds them as the scope's whole target.
func (s *Service) holdGoals(ctx context.Context, tx pgx.Tx, reach targetReach, frame crmcontracts.ReportingContext, keys []targetKey, held heldTargets) error {
	if frame.TargetInterval == nil || !auth.Allows(ctx, "sales_target", principal.ActionRead) {
		return nil
	}
	wanted := map[targetKey]goalFigure{}
	var owners []ids.UUID
	for _, key := range keys {
		_, explicit := held[key]
		_, seen := wanted[key]
		if key.metric != goalMetric || key.whole != (frame.PipelineId != nil) || explicit || seen {
			continue
		}
		figure, err := s.goalFigure(ctx, tx, reach, frame, key)
		if err != nil {
			return err
		}
		wanted[key] = figure
		owners = append(owners, figure.sellers...)
	}
	goals, whole, err := s.goalsOver(ctx, tx, owners, *frame.TargetInterval)
	if err != nil || !whole {
		return err
	}
	for key, figure := range wanted {
		if value, ok := goals.over(figure.sellers); ok {
			held[key] = goalTarget(frame, figure.scope, value)
		}
	}
	return nil
}

// goalFigure names the sellers whose goals add up to the figure of key, and
// none when the reader may not read every one of them.
func (s *Service) goalFigure(ctx context.Context, tx pgx.Tx, reach targetReach, frame crmcontracts.ReportingContext, key targetKey) (goalFigure, error) {
	if key.kind == reportingOwner {
		owner, err := ids.Parse(key.subject)
		if err != nil || !reach.owners.Includes(owner) {
			return goalFigure{}, err
		}
		id := openapi_types.UUID(owner)
		return goalFigure{scope: crmcontracts.ReportingScope{Kind: reportingOwner, Id: &id}, sellers: []ids.UUID{owner}}, nil
	}
	sellers, err := s.goalSellersAmong(ctx, tx, frame.Scope, reach.owners)
	if err != nil || sellers.Withheld {
		return goalFigure{}, err
	}
	return goalFigure{scope: frame.Scope, sellers: sellers.Owners}, nil
}
