// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package reporting

import (
	"context"
	"slices"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

var goalOrigins = []string{"manual", "ai", "adopted"}

// SaveGoalDraft stores the shared draft. It changes no reported figure, and the
// publication in force stays in force.
func (s *Service) SaveGoalDraft(ctx context.Context, owner ids.UUID, year int, version int64, in crmcontracts.SalesGoalDraftInput) (crmcontracts.SalesGoalPlan, error) {
	origin := "manual"
	if in.Origin != nil {
		origin = string(*in.Origin)
	}
	if err := checkGoalDraft(in, origin); err != nil {
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
		action := "update"
		after := goalPlan{}
		if before == nil {
			calendar, err := s.calendar(ctx, tx)
			if err != nil {
				return err
			}
			// The currency is fixed with the first draft; a later change of the
			// reporting currency refuses the publication rather than converting.
			after = goalPlan{ID: ids.NewV7(), OwnerID: owner, Year: year, Currency: calendar.Currency, CreatedAt: s.now()}
			action = "create"
			if err := insertGoalPlan(ctx, tx, goalPlan{ID: after.ID, OwnerID: owner, Year: year, Currency: after.Currency, Version: 1, CreatedAt: after.CreatedAt, UpdatedAt: after.CreatedAt}); err != nil {
				return storedError(err)
			}
		} else {
			after = *before
		}
		history, err := goalRevisions(ctx, tx, after.ID)
		if err != nil {
			return err
		}
		revision := goalRevision{Revision: nextRevision(history), Kind: goalKindDraft, Months: in.Months, Declared: in.DeclaredTotalMinor, Origin: origin, Reason: in.Reason, CreatedBy: author, CreatedAt: s.now()}
		if err := insertGoalRevision(ctx, tx, after.ID, revision); err != nil {
			return err
		}
		after.Draft, after.Version, after.UpdatedAt = &revision.Revision, after.Version+1, s.now()
		if err := updateGoalPlan(ctx, tx, after); err != nil {
			return err
		}
		out, err = s.afterWrite(ctx, tx, before, after, action)
		return err
	})
	return out, err
}

func checkGoalDraft(in crmcontracts.SalesGoalDraftInput, origin string) error {
	if len(in.Months) != goalMonths {
		return refuse("months", "invalid", "send twelve months, January to December")
	}
	if _, _, err := monthTotal(in.Months); err != nil {
		return err
	}
	if in.DeclaredTotalMinor != nil && (*in.DeclaredTotalMinor < 0 || *in.DeclaredTotalMinor > values.MaxExactInteger) {
		return refuse("declared_total_minor", "out_of_range", "enter a yearly goal of zero or more")
	}
	if !slices.Contains(goalOrigins, origin) {
		return refuse("origin", "invalid", "choose manual, ai or adopted")
	}
	return checkGoalReason(in.Reason)
}

// DiscardGoalDraft drops the draft into the history and keeps the publication.
func (s *Service) DiscardGoalDraft(ctx context.Context, owner ids.UUID, year int, version int64) (crmcontracts.SalesGoalPlan, error) {
	return s.closeGoalStep(ctx, owner, year, version, goalKindDiscarded, "")
}

// WithdrawGoalPlan retires the twelve published targets in one transaction, so
// Progress shows no goal until a new, explicit publication.
func (s *Service) WithdrawGoalPlan(ctx context.Context, owner ids.UUID, year int, version int64, reason string) (crmcontracts.SalesGoalPlan, error) {
	if err := checkGoalReason(reason); err != nil {
		return crmcontracts.SalesGoalPlan{}, err
	}
	return s.closeGoalStep(ctx, owner, year, version, goalKindWithdrawn, reason)
}

// closeGoalStep ends the draft (discard) or the publication (withdraw).
func (s *Service) closeGoalStep(ctx context.Context, owner ids.UUID, year int, version int64, kind, reason string) (crmcontracts.SalesGoalPlan, error) {
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
		ended := goalEnds(before, kind)
		if ended == nil {
			return refuse("plan", "nothing_to_"+kind, "this plan has no "+map[string]string{goalKindDiscarded: "draft", goalKindWithdrawn: "publication"}[kind])
		}
		history, err := goalRevisions(ctx, tx, before.ID)
		if err != nil {
			return err
		}
		closing := revisionAt(history, ended)
		if reason == "" {
			reason = closing.Reason
		}
		after := *before
		if kind == goalKindWithdrawn {
			if err := s.retireGoalTargets(ctx, tx, *before, closing.Targets, reason); err != nil {
				return err
			}
		}
		revision := goalRevision{Revision: nextRevision(history), Kind: kind, Months: closing.Months, Declared: closing.Declared, Origin: closing.Origin, Reason: reason, CreatedBy: author, CreatedAt: s.now()}
		if err := insertGoalRevision(ctx, tx, before.ID, revision); err != nil {
			return err
		}
		if kind == goalKindWithdrawn {
			after.Published, after.Withdrawn = nil, &revision.Revision
		} else {
			after.Draft = nil
		}
		after.Version, after.UpdatedAt = after.Version+1, s.now()
		if err := updateGoalPlan(ctx, tx, after); err != nil {
			return err
		}
		out, err = s.afterWrite(ctx, tx, before, after, closeAuditAction[kind])
		return err
	})
	return out, err
}

// closeAuditAction names a discard or withdrawal in the audit log's fixed verbs.
var closeAuditAction = map[string]string{goalKindDiscarded: "delete", goalKindWithdrawn: "revoke"}

// goalEnds is the revision a discard or withdrawal closes, nil when none.
func goalEnds(plan *goalPlan, kind string) *int64 {
	if plan == nil {
		return nil
	}
	if kind == goalKindWithdrawn {
		return plan.Published
	}
	return plan.Draft
}

func (s *Service) retireGoalTargets(ctx context.Context, tx pgx.Tx, plan goalPlan, links []goalTargetLink, reason string) error {
	existing, err := yearTargets(ctx, tx, []ids.UUID{plan.OwnerID}, plan.Year, true)
	if err != nil {
		return err
	}
	for _, link := range links {
		cell := existing[plan.OwnerID][link.Month]
		if _, err := s.publishMonth(ctx, tx, plan.OwnerID, plan.Year, link.Month, cell.Value, true, reason, existing[plan.OwnerID]); err != nil {
			return err
		}
	}
	return nil
}
