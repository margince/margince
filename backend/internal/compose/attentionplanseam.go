// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/margince/margince/backend/internal/compose/attention"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/weeklyplan"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	openapi_types "github.com/oapi-codegen/runtime/types"
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
	var zone string
	err = database.WithWorkspaceTx(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		zone, err = identity.TimezoneOf(ctx, tx)
		return err
	})
	if err != nil {
		return nil, err
	}
	location, err := time.LoadLocation(zone)
	if err != nil {
		return nil, err
	}
	out := []attention.PlanWork{}
	for _, commitment := range plan.Commitments {
		if commitment.State != "open" || commitment.DueOn == nil {
			continue
		}
		// A date-only promise remains on time throughout that date in the plan's zone.
		date := *commitment.DueOn
		due := time.Date(date.Year(), date.Month(), date.Day(), 23, 59, 59, 0, location)
		localNow := now.In(due.Location())
		tomorrow := time.Date(localNow.Year(), localNow.Month(), localNow.Day()+1, 0, 0, 0, 0, due.Location())
		if !due.Before(tomorrow) {
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
