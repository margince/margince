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
		return nil, nil
	}
	entity, err := storekit.LegalEntityOf(ctx, tx, user)
	if err != nil {
		return nil, err
	}
	return &entity, nil
}
