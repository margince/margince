// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"fmt"
	"log/slog"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// PurgeRemovedAdmitted is PurgeRemoved with admit asked first in the
// transaction that selects what the deletion destroys.
func (p *CapturePurger) PurgeRemovedAdmitted(
	ctx context.Context, admit func(context.Context, pgx.Tx) error, seat ids.UUID, provider, providerMessageID string,
) error {
	sourceCtx := ctx
	actor, bound := principal.Actor(ctx)
	mailboxDeletion := bound && actor.Type == principal.PrincipalConnector
	if mailboxDeletion && (admit == nil || actor.UserID != seat || actor.ID != "connector:"+provider) {
		return apperrors.ErrPermissionDenied
	}
	var subject capture.PurgeSubject
	if err := database.WithWorkspaceTx(ctx, p.pool, func(tx pgx.Tx) error {
		if admit != nil {
			if err := admit(ctx, tx); err != nil {
				return err
			}
		}
		floor, _, err := purgeFloorTx(ctx, tx)
		if err != nil {
			return err
		}
		subject, err = capture.SelectRemovedPurgeTx(ctx, tx, seat, provider, providerMessageID, floor)
		return err
	}); err != nil {
		return fmt.Errorf("capture: selecting what a mailbox-side deletion destroys: %w", err)
	}
	if subject.Total() == 0 {
		// Nothing captured under that key, or not by this seat. Most deleted
		// mail was never captured, so this is not an error.
		slog.InfoContext(ctx, "capture: mailbox removal has no captured provider mapping", "provider", provider, "unmatched_removals", 1)
		return nil
	}
	if len(subject.SoleImports) == 0 {
		slog.InfoContext(ctx, "capture: a mailbox-side deletion left the message standing",
			"provider", provider,
			"shared_with_colleagues", len(subject.SharedImports),
			"withheld", len(subject.Restricted))
		return nil
	}
	// No contacts are anonymised. A deletion speaks about one mail, and the
	// owner's exclusion rule is where a correspondent is judged.
	// Provider removal is maintenance of the admitted mailbox, not a grant to
	// delete arbitrary activities. The selection has proved the seat's sole copy.
	if mailboxDeletion {
		ctx = principal.WithActor(ctx, principal.Principal{Type: principal.PrincipalSystem, ID: "system:capture", UserID: seat, OnBehalfOf: seat})
	}
	check := p.removedStillPurgeable(sourceCtx, admit, seat, provider, providerMessageID)
	if _, err := p.retention.PurgeActivitiesIf(ctx, subject.SoleImports, privacy.PurgeMailboxDeletion, check); err != nil {
		return fmt.Errorf("capture: destroying a message the owner deleted at the provider: %w", err)
	}
	return nil
}

func (p *CapturePurger) removedStillPurgeable(sourceCtx context.Context, admit func(context.Context, pgx.Tx) error, seat ids.UUID, provider, providerMessageID string) privacy.ActivityStillPurgeable {
	return func(checkCtx context.Context, tx pgx.Tx, id ids.UUID) (bool, error) {
		if admit != nil {
			if err := admit(sourceCtx, tx); err != nil {
				return false, err
			}
		}
		floor, _, err := purgeFloorTx(checkCtx, tx)
		if err != nil {
			return false, err
		}
		current, err := capture.SelectRemovedPurgeTx(checkCtx, tx, seat, provider, providerMessageID, floor)
		if err != nil {
			return false, err
		}
		return slices.Contains(current.SoleImports, id), nil
	}
}
