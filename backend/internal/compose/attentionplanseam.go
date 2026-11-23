// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/attention"
	"github.com/margince/margince/backend/internal/compose/dayrule"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/weeklyplan"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/deadline"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type attentionWeeklyPlan struct {
	store *weeklyplan.Store
	pool  *pgxpool.Pool
}

func (s attentionWeeklyPlan) DuePlan(ctx context.Context, owner ids.UUID, now time.Time) ([]attention.PlanWork, error) {
	var plan weeklyplan.Plan
	var err error
	if owner.IsZero() {
		plan, err = s.store.Current(ctx, now)
	} else {
		plan, err = s.store.PlanFor(ctx, owner, now)
	}
	if errors.Is(err, apperrors.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if plan.Status != weeklyplan.StateOpen {
		return nil, nil
	}
	// The reader's own plan reads their day. A plan opened by id, including
	// the reader's own on a team screen, reads the installation's.
	var own *ids.UUID
	if owner.IsZero() {
		me := dayrule.ActingUser(ctx)
		own = &me
	}
	location, _, err := dayrule.ForAssignee(ctx, s.pool, own)
	if err != nil {
		return nil, err
	}
	out := []attention.PlanWork{}
	for _, commitment := range plan.Commitments {
		if commitment.State != weeklyplan.StateOpen || commitment.DueOn == nil {
			continue
		}
		// A date-only promise remains on time throughout that date in the plan's zone.
		date := *commitment.DueOn
		due := time.Date(date.Year(), date.Month(), date.Day(), 23, 59, 59, 0, location)
		localNow := now.In(due.Location())
		tomorrow := time.Date(localNow.Year(), localNow.Month(), localNow.Day()+1, 0, 0, 0, 0, due.Location())
		if !deadline.Passed(&due, tomorrow) {
			continue
		}
		row := attention.PlanWork{ID: commitment.ID, OwnerID: plan.OwnerID, Label: commitment.Label, DueAt: due}
		if !commitment.LinkedRecordID.IsZero() {
			row.Subject = &crmcontracts.AttentionSubject{Type: crmcontracts.AttentionSubjectType(commitment.LinkedRecordType), Id: openapi_types.UUID(commitment.LinkedRecordID)}
		}
		out = append(out, row)
	}
	return out, nil
}
