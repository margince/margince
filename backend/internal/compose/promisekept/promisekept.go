// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package promisekept closes our promises that a meeting kept. It acts only for
// a meeting an internal seat made, and only on contacts that seat may write.
package promisekept

import (
	"context"
	"errors"
	"log/slog"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ActingAs builds the context a write runs under for an actor.
type ActingAs func(ctx context.Context, by events.Actor) (context.Context, bool, error)

// Settle closes the promises an accepted guest of this meeting kept. The system
// writes it, so the audit row says it was automatic and names the meeting.
func Settle(ctx context.Context, claims *contacts.Store, acting ActingAs, log *slog.Logger, meeting ids.UUID) error {
	asSystem, ok, err := acting(ctx, events.Actor{Type: string(principal.PrincipalSystem)})
	if err != nil || !ok {
		return err
	}
	seat, internal, err := claims.MeetingSeat(asSystem, meeting)
	if err != nil || !internal {
		return err
	}
	asSeat, ok, err := acting(ctx, events.Actor{
		Type: string(principal.PrincipalHuman), ID: principal.HumanIDPrefix + seat.String(),
	})
	if err != nil || !ok {
		return err
	}
	kept, err := claims.KeptByAcceptedGuest(asSystem, meeting)
	if err != nil {
		return err
	}
	for _, claim := range kept {
		if err := claims.SettleKeptByEvidence(asSystem, asSeat, claim, meeting); err != nil {
			if errors.Is(err, apperrors.ErrPermissionDenied) || errors.Is(err, apperrors.ErrNotFound) {
				log.WarnContext(ctx, "kept promise settlement refused", "claim", claim, "err", err)
				continue
			}
			return err
		}
	}
	return nil
}
