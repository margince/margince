// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatlinks

import (
	"context"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func requireRecordRead(ctx context.Context, kind string) error {
	switch kind {
	case RecordCompany:
		return auth.Require(ctx, RecordCompany, principal.ActionRead)
	case RecordDeal:
		return auth.Require(ctx, RecordDeal, principal.ActionRead)
	case RecordProject:
		return auth.Require(ctx, RecordProject, principal.ActionRead)
	}
	return apperrors.ErrInvalidArgument
}

func requireRecordUpdate(ctx context.Context, kind string) error {
	switch kind {
	case RecordCompany:
		return auth.Require(ctx, RecordCompany, principal.ActionUpdate)
	case RecordDeal:
		return auth.Require(ctx, RecordDeal, principal.ActionUpdate)
	case RecordProject:
		return auth.Require(ctx, RecordProject, principal.ActionUpdate)
	}
	return apperrors.ErrInvalidArgument
}
