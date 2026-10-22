// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package projects

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func admitProjectCreation(ctx context.Context, tx pgx.Tx, in CreateProjectInput, fallback *ids.UserID) (CreateProjectInput, error) {
	owner, err := storekit.NewRecordOwnerWithDefault(ctx, tx, in.OwnerID, fallback)
	if err != nil {
		return CreateProjectInput{}, err
	}
	in.OwnerID = owner
	if err := auth.EnsureReferenceTarget(ctx, tx, "company_id", "company", in.CompanyID.UUID); err != nil {
		return CreateProjectInput{}, err
	}
	if err := storekit.RefuseUnknownSeat(ctx, tx, in.Author); err != nil {
		return CreateProjectInput{}, err
	}
	return in, nil
}

// PreviewProjectCreationForActor leaves key allocation to the writer rather than reserving it in review.
func (s *Store) PreviewProjectCreationForActor(ctx context.Context, in CreateProjectInput, creator principal.Principal) (CreateProjectInput, error) {
	if err := auth.Require(ctx, projectObject, principal.ActionCreate); err != nil {
		return CreateProjectInput{}, err
	}
	creatorCtx := principal.WithActor(ctx, creator)
	if err := auth.Require(creatorCtx, projectObject, principal.ActionCreate); err != nil {
		return CreateProjectInput{}, err
	}
	if _, err := storekit.CapturedBy(creatorCtx); err != nil {
		return CreateProjectInput{}, err
	}
	active, err := s.catalogColumns(ctx)
	if err != nil {
		return CreateProjectInput{}, err
	}
	var out CreateProjectInput
	err = s.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		out, err = admitProjectCreation(ctx, tx, in, storekit.OwnerOrActor(creatorCtx))
		return err
	})
	if err != nil {
		return CreateProjectInput{}, err
	}
	out.Key = nil
	out.CustomFields, err = storekit.CreateFieldValues(active, in.CustomFields)
	return out, err
}
