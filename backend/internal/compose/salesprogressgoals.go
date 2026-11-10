// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"slices"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/reporting"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// progressPeople is whose goals and figures a reading adds up.
type progressPeople struct {
	team bool
	// owners are the people whose goals count, each one the reader measures.
	owners []ids.UUID
	// withheld is a team the reader may not measure member by member: its
	// goal is the members' goals added up, so it is withheld with them.
	withheld bool
}

func (h salesProgressHandlers) people(ctx context.Context, tx pgx.Tx, frame crmcontracts.ReportingContext) (progressPeople, error) {
	if frame.Scope.Kind == crmcontracts.ReportingScopeKindOwner && frame.Scope.Id != nil {
		return progressPeople{owners: []ids.UUID{ids.UUID(*frame.Scope.Id)}}, nil
	}
	members, err := reportingMembers(ctx, tx, frame.Scope)
	if err != nil {
		return progressPeople{}, err
	}
	measured, err := readerMeasurableOwners(ctx, tx)
	if err != nil {
		return progressPeople{}, err
	}
	out := progressPeople{team: true}
	for _, member := range members {
		if measured.Includes(member) && !measured.IndividualOff {
			out.owners = append(out.owners, member)
		} else {
			out.withheld = true
		}
	}
	return out, nil
}

// progressGoals holds the goals a reading compares against.
type progressGoals struct {
	readable bool
	withheld bool
	currency string
	years    map[ids.UUID]reporting.GoalYear
	drift    map[ids.UUID]bool
}

func (h salesProgressHandlers) goals(ctx context.Context, tx pgx.Tx, frame progressFrame, people progressPeople) (progressGoals, error) {
	out := progressGoals{withheld: people.withheld, currency: frame.currency}
	var err error
	out.years, out.readable, err = h.reports.OwnerGoalYears(ctx, tx, people.owners, frame.period.year)
	if err != nil || !out.readable {
		return out, err
	}
	out.drift, err = h.reports.GoalPlanDrift(ctx, tx, people.owners, frame.period.year)
	return out, err
}

// over is the goal of owners across months. yearly withholds the comparison
// from a plan another route changed, until a manager reviews it.
func (g progressGoals) over(owners []ids.UUID, months []int, yearly bool) crmcontracts.SalesProgressGoal {
	out := crmcontracts.SalesProgressGoal{Status: "missing", MonthsNeeded: len(months)}
	if !g.readable || g.withheld {
		out.Status = "withheld"
		return out
	}
	var total int64
	known, foreign := false, false
	for _, month := range months {
		complete := len(owners) > 0
		for _, owner := range owners {
			cell, ok := g.years[owner][month]
			switch {
			case !ok:
				complete = false
			case cell.Unit != g.currency:
				foreign, complete = true, false
			case total > values.MaxExactInteger-cell.Value:
				out.Status = "withheld"
				return out
			default:
				total += cell.Value
				known = true
			}
		}
		if complete {
			out.MonthsSet++
		}
	}
	switch {
	case foreign:
		out.Status = "other_currency"
		return out
	case known:
		out.AmountMinor = &total
	}
	switch {
	case !known:
	case out.MonthsSet < len(months):
		out.Status = "incomplete"
	case yearly && slices.ContainsFunc(owners, func(owner ids.UUID) bool { return g.drift[owner] }):
		out.Status = "changed_outside_plan"
	case total == 0:
		out.Status = "zero"
	default:
		out.Status = "set"
	}
	return out
}

// compared fills completion and delta, which only a complete goal over a
// complete amount supports. A zero goal has a delta and no completion.
func compared(closed crmcontracts.SalesProgressAmount, goal crmcontracts.SalesProgressGoal) (completion *float64, delta *int64) {
	if closed.Status != "ok" || closed.AmountMinor == nil || goal.AmountMinor == nil || (goal.Status != "set" && goal.Status != "zero") {
		return nil, nil
	}
	difference := *closed.AmountMinor - *goal.AmountMinor
	delta = &difference
	if goal.Status == "set" {
		ratio := float64(*closed.AmountMinor) / float64(*goal.AmountMinor)
		completion = &ratio
	}
	return completion, delta
}
