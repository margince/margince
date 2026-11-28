// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// WithAuthority shares live actor reconstruction with channel connection operations.
func (s *ChannelStore) WithAuthority(resolve func(context.Context, pgx.Tx) (context.Context, error)) *ChannelStore {
	next := *s
	next.authority = resolve
	return &next
}

func (s *ChannelStore) writeAuthority(ctx context.Context, tx pgx.Tx, action principal.Action) (context.Context, error) {
	if s.authority != nil {
		var err error
		ctx, err = s.authority(ctx, tx)
		if err != nil {
			return nil, err
		}
	}
	if !auth.SeatAdmitsWrites(ctx) {
		return nil, apperrors.ErrSeatTierInsufficient
	}
	if err := auth.RequireHuman(ctx); err != nil {
		return nil, err
	}
	var err error
	switch action {
	case principal.ActionCreate:
		err = auth.Require(ctx, channelConnectionObject, principal.ActionCreate)
	case principal.ActionUpdate:
		err = auth.Require(ctx, channelConnectionObject, principal.ActionUpdate)
	default:
		return nil, apperrors.ErrInvalidArgument
	}
	if err != nil {
		return nil, err
	}
	return ctx, nil
}

func channelAuthorityRefused(err error) bool {
	return errors.Is(err, apperrors.ErrNotFound) || errors.Is(err, apperrors.ErrPermissionDenied) || errors.Is(err, apperrors.ErrSeatTierInsufficient)
}
