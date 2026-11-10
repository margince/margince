// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func loggedCorrespondenceEntity(ctx context.Context, tx pgx.Tx, in LogActivityInput, host *ids.UserID) (*ids.UUID, error) {
	if !in.correspondenceEntity.IsZero() {
		return &in.correspondenceEntity, nil
	}
	user := storekit.SenderOf(ctx)
	if user.IsZero() && host != nil {
		user = host.UUID
	}
	if user.IsZero() {
		return nil, nil //nolint:nilnil // automatic work with no seat names no company; the nil binds as a NULL legal_entity_id
	}
	entity, err := storekit.LegalEntityOf(ctx, tx, user)
	if err != nil {
		return nil, err
	}
	return &entity, nil
}

func prepareLoggedCorrespondence(ctx context.Context, tx pgx.Tx, in LogActivityInput, host *ids.UserID) (string, *ids.UUID, error) {
	counterparty, err := counterpartyFor(ctx, tx, in)
	if err != nil {
		return "", nil, err
	}
	if err := lockCorrespondenceForAttestedSend(ctx, tx, in, counterparty); err != nil {
		return "", nil, err
	}
	if err := storekit.RefuseUnknownSeat(ctx, tx, in.Author); err != nil {
		return "", nil, err
	}
	entity, err := loggedCorrespondenceEntity(ctx, tx, in, host)
	return counterparty, entity, err
}
