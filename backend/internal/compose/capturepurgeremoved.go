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
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// removedSelector answers what a provider's deletion of one message destroys,
// under the transaction and floor the purge reads it in.
type removedSelector func(ctx context.Context, tx pgx.Tx, floor capture.StatutoryFloor) (capture.PurgeSubject, error)

// PurgeRemovedAdmitted is PurgeRemoved with admit asked first in the
// transaction that selects what the deletion destroys.
func (p *CapturePurger) PurgeRemovedAdmitted(
	ctx context.Context, admit func(context.Context, pgx.Tx) error, seat ids.UUID, provider, providerMessageID string,
) error {
	return p.purgeRemovedWith(ctx, admit, seat, "connector:"+provider, mailboxRemovalSelector(seat, provider, providerMessageID))
}

func mailboxRemovalSelector(seat ids.UUID, provider, providerMessageID string) removedSelector {
	return func(ctx context.Context, tx pgx.Tx, floor capture.StatutoryFloor) (capture.PurgeSubject, error) {
		return capture.SelectRemovedPurgeTx(ctx, tx, seat, provider, providerMessageID, floor)
	}
}

// PurgeRemovedChat applies a chat provider's deletion of one message under the
// rules a mailbox deletion follows.
func (p *CapturePurger) PurgeRemovedChat(
	ctx context.Context, admit func(context.Context, pgx.Tx) error, seat ids.UUID, key connector.NaturalKey,
) error {
	actor, bound := principal.Actor(ctx)
	if !bound || actor.Type != principal.PrincipalConnector {
		return apperrors.ErrPermissionDenied
	}
	return p.purgeRemovedWith(ctx, admit, seat, actor.ID,
		func(ctx context.Context, tx pgx.Tx, floor capture.StatutoryFloor) (capture.PurgeSubject, error) {
			return capture.SelectRemovedChatPurgeTx(ctx, tx, seat, actor.ID, key, floor)
		})
}

// purgeRemovedWith destroys the seat's sole copy a provider deletion names. A
// connector's deletion needs admit and its own seat, because it is maintenance
// of an admitted account and no grant to delete arbitrary activities.
func (p *CapturePurger) purgeRemovedWith(
	ctx context.Context, admit func(context.Context, pgx.Tx) error, seat ids.UUID, connectorID string, selectRemoved removedSelector,
) error {
	sourceCtx := ctx
	actor, bound := principal.Actor(ctx)
	mailboxDeletion := bound && actor.Type == principal.PrincipalConnector
	if mailboxDeletion && (admit == nil || actor.UserID != seat || actor.ID != connectorID) {
		return apperrors.ErrPermissionDenied
	}
	var subject capture.PurgeSubject
	if err := database.WithTx(ctx, p.pool, func(tx pgx.Tx) error {
		if admit != nil {
			if err := admit(ctx, tx); err != nil {
				return err
			}
		}
		floor, _, err := purgeFloorTx(ctx, tx)
		if err != nil {
			return err
		}
		subject, err = selectRemoved(ctx, tx, floor)
		return err
	}); err != nil {
		return fmt.Errorf("capture: selecting what a provider-side deletion destroys: %w", err)
	}
	if subject.Total() == 0 {
		// Nothing captured under that key, or not by this seat. Most deleted
		// messages were never captured, so this is not an error.
		slog.InfoContext(ctx, "capture: provider removal has no captured mapping", "unmatched_removals", 1)
		return nil
	}
	if len(subject.SoleImports) == 0 {
		slog.InfoContext(ctx, "capture: a provider-side deletion left the message standing",
			"shared_with_colleagues", len(subject.SharedImports),
			"withheld", len(subject.Restricted))
		return nil
	}
	// No contacts are anonymised. A deletion speaks about one message, and the
	// owner's exclusion rule is where a correspondent is judged.
	// Provider removal is maintenance of the admitted account, not a grant to
	// delete arbitrary activities. The selection has proved the seat's sole copy.
	if mailboxDeletion {
		ctx = principal.WithActor(ctx, principal.Principal{Type: principal.PrincipalSystem, ID: "system:capture", UserID: seat, OnBehalfOf: seat})
	}
	check := removedStillPurgeableBy(sourceCtx, admit, selectRemoved)
	if _, err := p.retention.PurgeActivitiesIf(ctx, subject.SoleImports, privacy.PurgeMailboxDeletion, check); err != nil {
		return fmt.Errorf("capture: destroying a message the owner deleted at the provider: %w", err)
	}
	return nil
}

func (p *CapturePurger) removedStillPurgeable(sourceCtx context.Context, admit func(context.Context, pgx.Tx) error, seat ids.UUID, provider, providerMessageID string) privacy.ActivityStillPurgeable {
	return removedStillPurgeableBy(sourceCtx, admit, mailboxRemovalSelector(seat, provider, providerMessageID))
}

func removedStillPurgeableBy(sourceCtx context.Context, admit func(context.Context, pgx.Tx) error, selectRemoved removedSelector) privacy.ActivityStillPurgeable {
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
		current, err := selectRemoved(checkCtx, tx, floor)
		if err != nil {
			return false, err
		}
		return slices.Contains(current.SoleImports, id), nil
	}
}
