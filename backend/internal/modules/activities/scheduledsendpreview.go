// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func (s *Store) ReadScheduledSendForDecision(ctx context.Context, id, initiatingSeat ids.UUID) (ScheduledSend, error) {
	actor, ok := principal.Actor(ctx)
	if !ok || actor.Type != principal.PrincipalHuman {
		return ScheduledSend{}, apperrors.ErrPermissionDenied
	}
	if err := auth.Require(ctx, "communication_exception", principal.ActionRead); err != nil {
		return ScheduledSend{}, err
	}
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return ScheduledSend{}, err
	}
	var out ScheduledSend
	err := s.tx(ctx, func(tx pgx.Tx) error {
		var err error
		out, err = readScheduledSendForSeatTx(ctx, tx, id, initiatingSeat)
		return err
	})
	return out, err
}

func (message ScheduledSend) PreviewMessage() (SendEmailInput, []ActivityLinkInput, error) {
	if message.PayloadVersion != payloadVersionCurrent {
		return SendEmailInput{}, nil, apperrors.ErrInvalidArgument
	}
	input, err := message.frozen.thaw()
	if err != nil {
		return SendEmailInput{}, nil, err
	}
	links, err := thawOriginLinks(message.additionalLinks)
	return input, links, err
}
