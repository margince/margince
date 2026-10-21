// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// PreviewCreateDealTx applies the writer's defaults and preconditions without
// inserting a deal or its history, audit and event rows.
func (s *Store) PreviewCreateDealTx(ctx context.Context, tx pgx.Tx, in CreateDealInput) (CreateDealInput, error) {
	if err := auth.Require(ctx, "deal", principal.ActionCreate); err != nil {
		return CreateDealInput{}, err
	}
	if len(in.CustomFields) != 0 {
		return CreateDealInput{}, ErrCustomFieldsNeedTheStoresOwnTransaction
	}
	born, err := s.readyDealCreate(ctx, in)
	if err != nil {
		return CreateDealInput{}, err
	}
	out, err := s.clearedForBirth(ctx, tx, in, born.pastCloseAllowed)
	if err != nil {
		return CreateDealInput{}, err
	}
	out.PartnerAttribution = born.attribution
	return out, nil
}

// PreviewDealCreationForActor takes defaults from the creator but reads references as the reviewer.
func (s *Store) PreviewDealCreationForActor(ctx context.Context, in CreateDealInput, creator principal.Principal) (CreateDealInput, error) {
	return s.previewActorDealCreation(ctx, in, creator, nil)
}

// PreviewImportDealCreationForActor applies import terminal-state rules without creating the deal.
func (s *Store) PreviewImportDealCreationForActor(ctx context.Context, in CreateDealInput, creator principal.Principal, terminal *ImportTerminal) (CreateDealInput, error) {
	return s.previewActorDealCreation(ctx, in, creator, terminal)
}

func (s *Store) previewActorDealCreation(ctx context.Context, in CreateDealInput, creator principal.Principal, terminal *ImportTerminal) (CreateDealInput, error) {
	if err := auth.Require(ctx, "deal", principal.ActionCreate); err != nil {
		return CreateDealInput{}, err
	}
	creatorCtx := principal.WithActor(ctx, creator)
	if err := auth.Require(creatorCtx, "deal", principal.ActionCreate); err != nil {
		return CreateDealInput{}, err
	}
	born, err := s.readyDealCreate(creatorCtx, in)
	if err != nil {
		return CreateDealInput{}, err
	}
	active, err := s.activeColumns(ctx)
	if err != nil {
		return CreateDealInput{}, err
	}
	var out CreateDealInput
	err = s.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		if terminal != nil {
			in.StageID, err = s.RefuseImportBirth(ctx, tx, in, terminal)
			if err != nil {
				return err
			}
			born.pastCloseAllowed = true
		}
		out, err = s.clearedForBirthWithOwner(ctx, tx, in, born.pastCloseAllowed, storekit.OwnerOrActor(creatorCtx))
		return err
	})
	if err != nil {
		return CreateDealInput{}, err
	}
	out.PartnerAttribution = born.attribution
	if terminal != nil {
		out.StageID = terminal.StageID
	}
	out.CustomFields = storekit.CreateFieldValues(active, in.CustomFields)
	return out, nil
}
