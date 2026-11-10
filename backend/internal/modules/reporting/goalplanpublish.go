// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package reporting

import (
	"context"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// PublishGoalPlan makes the draft the agreed goals. Twelve monthly targets, the
// publication and their audit rows commit together. If one fails, none does and
// the previous publication stays in force.
func (s *Service) PublishGoalPlan(ctx context.Context, owner ids.UUID, year int, version int64, in crmcontracts.SalesGoalPublishInput) (crmcontracts.SalesGoalPlan, error) {
	if !in.ConfirmAgreed {
		return crmcontracts.SalesGoalPlan{}, refuse("confirm_agreed", "required", "confirm these are the goals you agreed together")
	}
	if err := checkGoalReason(in.Reason); err != nil {
		return crmcontracts.SalesGoalPlan{}, err
	}
	author, err := actorID(ctx)
	if err != nil {
		return crmcontracts.SalesGoalPlan{}, err
	}
	var out crmcontracts.SalesGoalPlan
	err = s.store.db.Tx(ctx, func(tx pgx.Tx) error {
		before, err := s.planForWrite(ctx, tx, owner, year, version)
		if err != nil {
			return err
		}
		if before == nil || before.Draft == nil {
			return refuse("plan", "no_draft", "save a draft before you publish")
		}
		history, err := goalRevisions(ctx, tx, before.ID)
		if err != nil {
			return err
		}
		draft := revisionAt(history, before.Draft)
		if err := s.checkPublishable(ctx, tx, *before, *draft); err != nil {
			return err
		}
		links, err := s.writeGoalMonths(ctx, tx, *before, draft.Months, in.Reason)
		if err != nil {
			return err
		}
		revision := goalRevision{Revision: nextRevision(history), Kind: goalKindPublished, Months: draft.Months, Declared: draft.Declared, Origin: draft.Origin, Reason: in.Reason, Targets: links, CreatedBy: author, CreatedAt: s.now()}
		if err := insertGoalRevision(ctx, tx, before.ID, revision); err != nil {
			return err
		}
		after := *before
		after.Published, after.Draft, after.Withdrawn = &revision.Revision, nil, nil
		after.Version, after.UpdatedAt = after.Version+1, s.now()
		if err := updateGoalPlan(ctx, tx, after); err != nil {
			return err
		}
		out, err = s.afterWrite(ctx, tx, before, after, "publish")
		return err
	})
	return out, err
}

// checkPublishable holds the rules a draft may break and a publication may not.
func (s *Service) checkPublishable(ctx context.Context, tx pgx.Tx, plan goalPlan, draft goalRevision) error {
	total, complete, err := monthTotal(draft.Months)
	if err != nil {
		return err
	}
	if !complete {
		return refuse("months", "incomplete", "set every month before you publish; a month may be zero")
	}
	if total <= 0 {
		return refuse("months", "zero_total", "the yearly goal must be more than zero")
	}
	if draft.Declared != nil && *draft.Declared != total {
		return refuse("declared_total_minor", "mismatch", "the months must add up to the yearly goal")
	}
	calendar, err := s.calendar(ctx, tx)
	if err != nil {
		return err
	}
	if calendar.Currency != plan.Currency {
		return refuse("currency", "other_currency", "this plan is kept in "+plan.Currency+" and reporting now uses "+calendar.Currency+"; start a new draft after the reporting currency is settled")
	}
	return nil
}

// writeGoalMonths writes each month through the target writers and returns the
// twelve revisions the publication links to.
func (s *Service) writeGoalMonths(ctx context.Context, tx pgx.Tx, plan goalPlan, months []*int64, reason string) ([]goalTargetLink, error) {
	existing, err := yearTargets(ctx, tx, []ids.UUID{plan.OwnerID}, plan.Year, true)
	if err != nil {
		return nil, err
	}
	links := make([]goalTargetLink, 0, goalMonths)
	for index, value := range months {
		month := index + 1
		if cell, ok := existing[plan.OwnerID][month]; ok && cell.Unit != plan.Currency {
			return nil, refuse("months", "other_currency", "a target for this month is kept in "+cell.Unit+"; correct it before you publish")
		}
		link, err := s.publishMonth(ctx, tx, plan.OwnerID, plan.Year, month, *value, false, reason, existing[plan.OwnerID])
		if err != nil {
			return nil, err
		}
		links = append(links, link)
	}
	return links, nil
}

// ListGoalSellers lists the caller and every colleague whose goals they may read.
func (s *Service) ListGoalSellers(ctx context.Context, year int) (crmcontracts.SalesGoalRoster, error) {
	var out crmcontracts.SalesGoalRoster
	err := s.store.db.Tx(ctx, func(tx pgx.Tx) error {
		self, err := actorID(ctx)
		if err != nil {
			return err
		}
		if _, err := s.goalAccess(ctx, tx, self); err != nil {
			return err
		}
		calendar, err := s.calendar(ctx, tx)
		if err != nil {
			return err
		}
		owners, err := s.authority.MembersFor(ctx, tx, crmcontracts.ReportingScope{})
		if err != nil {
			return err
		}
		measured, err := s.authority.MeasurableOwners(ctx, tx, owners)
		if err != nil {
			return err
		}
		out = crmcontracts.SalesGoalRoster{Year: year, Currency: calendar.Currency}
		out.Data, err = s.goalSellers(ctx, tx, self, measured, year)
		return err
	})
	return out, err
}

func (s *Service) goalSellers(ctx context.Context, tx pgx.Tx, self ids.UUID, measured []ids.UUID, year int) ([]crmcontracts.SalesGoalSeller, error) {
	people := append([]ids.UUID{self}, measured...)
	rows, err := tx.Query(ctx, `SELECT u.id, COALESCE(u.display_name,''), p.version IS NOT NULL, p.draft_revision IS NOT NULL, p.published_revision IS NOT NULL, p.withdrawn_revision IS NOT NULL
		FROM app_user u LEFT JOIN sales_goal_plan p ON p.owner_id=u.id AND p.year=$2
		WHERE u.id=ANY($1) ORDER BY u.id=$3 DESC, u.display_name, u.id`, people, year, self)
	if err != nil {
		return nil, err
	}
	type row struct {
		id                                  ids.UUID
		name                                string
		exists, draft, published, withdrawn bool
	}
	found, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (row, error) {
		var out row
		return out, r.Scan(&out.id, &out.name, &out.exists, &out.draft, &out.published, &out.withdrawn)
	})
	if err != nil {
		return nil, err
	}
	drift, err := s.GoalPlanDrift(ctx, tx, people, year)
	if err != nil {
		return nil, err
	}
	live, err := yearTargets(ctx, tx, people, year, false)
	if err != nil {
		return nil, err
	}
	calendar, err := s.calendar(ctx, tx)
	if err != nil {
		return nil, err
	}
	out := make([]crmcontracts.SalesGoalSeller, 0, len(found))
	for _, person := range found {
		header := goalPlan{}
		one := int64(1)
		if person.draft {
			header.Draft = &one
		}
		if person.published {
			header.Published = &one
		}
		if person.withdrawn {
			header.Withdrawn = &one
		}
		edit, err := s.goalWriteReachesCaller(ctx, tx, person.id)
		if err != nil {
			return nil, err
		}
		entry := crmcontracts.SalesGoalSeller{OwnerId: openapi_types.UUID(person.id), Name: person.name, State: goalPlanState(header, drift[person.id]), CanEdit: edit}
		if total, complete, err := monthTotal(liveMonths(live[person.id], calendar.Currency)); err == nil && complete {
			entry.TotalMinor = &total
		}
		out = append(out, entry)
	}
	return out, nil
}
