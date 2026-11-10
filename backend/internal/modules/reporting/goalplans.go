// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package reporting

import (
	"context"
	"strconv"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

const goalMonths = 12

// GetGoalPlan reads one person's plan for a year as the caller may see it.
func (s *Service) GetGoalPlan(ctx context.Context, owner ids.UUID, year int) (crmcontracts.SalesGoalPlan, error) {
	var out crmcontracts.SalesGoalPlan
	err := s.store.db.Tx(ctx, func(tx pgx.Tx) error {
		access, err := s.goalAccess(ctx, tx, owner)
		if err != nil {
			return err
		}
		out, err = s.goalPlanView(ctx, tx, owner, year, access)
		return err
	})
	return out, err
}

// goalPlanView builds the answer every plan route returns.
func (s *Service) goalPlanView(ctx context.Context, tx pgx.Tx, owner ids.UUID, year int, access goalAccess) (crmcontracts.SalesGoalPlan, error) {
	calendar, err := s.calendar(ctx, tx)
	if err != nil {
		return crmcontracts.SalesGoalPlan{}, err
	}
	out := crmcontracts.SalesGoalPlan{OwnerId: openapi_types.UUID(owner), Year: year, Currency: calendar.Currency, State: "no_goal", History: []crmcontracts.SalesGoalEvent{}}
	out.Capabilities.Edit = access.edit
	if err := tx.QueryRow(ctx, "SELECT COALESCE(display_name,'') FROM app_user WHERE id=$1", owner).Scan(&out.OwnerName); err != nil {
		return out, storedError(err)
	}
	live, err := yearTargets(ctx, tx, []ids.UUID{owner}, year, false)
	if err != nil {
		return out, err
	}
	out.LiveMonths = liveMonths(live[owner], calendar.Currency)
	plan, err := loadGoalPlan(ctx, tx, owner, year)
	if err != nil || plan == nil {
		return out, err
	}
	out.Currency, out.Version = plan.Currency, plan.Version
	history, err := goalRevisions(ctx, tx, plan.ID)
	if err != nil {
		return out, err
	}
	drift, err := s.GoalPlanDrift(ctx, tx, []ids.UUID{owner}, year)
	if err != nil {
		return out, err
	}
	out.State = goalPlanState(*plan, drift[owner])
	if published := revisionAt(history, plan.Published); published != nil {
		view := revisionView(*published)
		out.Published = &view
	}
	if !access.drafts {
		return out, nil
	}
	if draft := revisionAt(history, plan.Draft); draft != nil {
		view := revisionView(*draft)
		out.Draft = &view
	}
	for _, step := range history {
		out.History = append(out.History, eventView(step))
	}
	return out, nil
}

func goalPlanState(plan goalPlan, drifted bool) crmcontracts.SalesGoalPlanState {
	switch {
	case plan.Published != nil && drifted:
		return "changed_outside_plan"
	case plan.Published != nil && plan.Draft != nil:
		return "published_with_draft"
	case plan.Published != nil:
		return "published"
	case plan.Draft != nil:
		return "draft"
	case plan.Withdrawn != nil:
		return "withdrawn"
	}
	return "no_goal"
}

// liveMonths is the reported goal of each month, in the plan currency only.
// A target kept in another currency is never shown as if it were this one.
func liveMonths(year GoalYear, currency string) crmcontracts.SalesGoalMonths {
	out := make(crmcontracts.SalesGoalMonths, goalMonths)
	for month, cell := range year {
		if cell.Unit == currency && !cell.Retired {
			value := cell.Value
			out[month-1] = &value
		}
	}
	return out
}

func revisionView(r goalRevision) crmcontracts.SalesGoalRevision {
	return crmcontracts.SalesGoalRevision{
		Revision: r.Revision, Months: r.Months, DeclaredTotalMinor: r.Declared, Reason: r.Reason,
		AuthorName: r.AuthorName, CreatedAt: r.CreatedAt, Origin: crmcontracts.SalesGoalRevisionOrigin(r.Origin),
	}
}

func eventView(r goalRevision) crmcontracts.SalesGoalEvent {
	out := crmcontracts.SalesGoalEvent{Revision: r.Revision, Kind: crmcontracts.SalesGoalEventKind(r.Kind), Reason: r.Reason, AuthorName: r.AuthorName, CreatedAt: r.CreatedAt}
	if total, complete, err := monthTotal(r.Months); complete && err == nil {
		out.TotalMinor = &total
	}
	return out
}

// The years a plan may cover, as the sales_goal_plan CHECK holds them.
const (
	minGoalYear = 2000
	maxGoalYear = 2200
)

// monthTotal adds twelve months. complete is false when a month is not set.
// A sum past the exact JSON range is refused rather than rounded.
func monthTotal(months []*int64) (total int64, complete bool, err error) {
	complete = len(months) == goalMonths
	for index, month := range months {
		if month == nil {
			complete = false
			continue
		}
		if *month < 0 || *month > values.MaxExactInteger {
			return 0, false, refuse("months["+strconv.Itoa(index)+"]", "out_of_range", "enter an amount of zero or more")
		}
		if total > values.MaxExactInteger-*month {
			return 0, false, refuse("months", "out_of_range", "the yearly goal is too large to keep exactly")
		}
		total += *month
	}
	return total, complete, nil
}

// checkGoalReason is the reason every plan step records.
func checkGoalReason(reason string) error {
	if !values.HasVisibleText(reason) {
		return refuse("reason", "required", "enter a reason")
	}
	if utf8.RuneCountInString(reason) > 1000 {
		return refuse("reason", "too_long", "a reason holds at most 1000 characters")
	}
	return nil
}

// planForWrite locks the person's year and checks the version the caller saw;
// version 0 means the caller saw no plan.
func (s *Service) planForWrite(ctx context.Context, tx pgx.Tx, owner ids.UUID, year int, version int64) (*goalPlan, error) {
	if year < minGoalYear || year > maxGoalYear {
		return nil, refuse("year", "out_of_range", "choose a year from 2000 to 2200")
	}
	if err := s.requireGoalEdit(ctx, tx, owner); err != nil {
		return nil, err
	}
	if err := lockGoalYear(ctx, tx, owner, year); err != nil {
		return nil, err
	}
	plan, err := loadGoalPlan(ctx, tx, owner, year)
	if err != nil {
		return nil, err
	}
	current := int64(0)
	if plan != nil {
		current = plan.Version
	}
	if current != version {
		return nil, apperrors.ErrVersionSkew
	}
	return plan, nil
}

// afterWrite records the header change and answers with the plan as it now is.
func (s *Service) afterWrite(ctx context.Context, tx pgx.Tx, before *goalPlan, after goalPlan, action string) (crmcontracts.SalesGoalPlan, error) {
	if err := recordChange(ctx, tx, "sales_goal_plan", after.ID, action, before, after); err != nil {
		return crmcontracts.SalesGoalPlan{}, err
	}
	access, err := s.goalAccess(ctx, tx, after.OwnerID)
	if err != nil {
		return crmcontracts.SalesGoalPlan{}, err
	}
	return s.goalPlanView(ctx, tx, after.OwnerID, after.Year, access)
}
