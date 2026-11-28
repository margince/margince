// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// DismissSilence records the owner's answer for the observed delivery episode.
// It uses the audit-only connection lifecycle because the event catalog has no connection entity.
func (r *Registry) DismissSilence(ctx context.Context, mb Mailbox, since time.Time) (ConnectionView, error) {
	actor, ok := principal.Actor(ctx)
	if !ok || !actor.HumanOrItsAgent() {
		return ConnectionView{}, apperrors.ErrPermissionDenied
	}
	provider := mb.Provider
	match, err := r.resolveMemberMailbox(ctx, actor.UserID, mb)
	if err != nil {
		return ConnectionView{}, err
	}
	err = r.db.Tx(ctx, func(tx pgx.Tx) error { return r.dismissSilenceTx(ctx, tx, match, provider, since) })
	if errors.Is(err, pgx.ErrNoRows) {
		return ConnectionView{}, ErrNoConnection
	}
	if err != nil {
		return ConnectionView{}, fmt.Errorf("capture: dismissing mailbox silence: %w", err)
	}
	return r.connectionFor(ctx, match.id)
}

// dismissSilenceTx records the dismissal on the locked row, when the episode
// the owner saw is still the one the row is in.
func (r *Registry) dismissSilenceTx(ctx context.Context, tx pgx.Tx, match connectionMatch, provider string, since time.Time) error {
	var view ConnectionView
	if err := tx.QueryRow(ctx, `SELECT id, provider, status, connected_at, silence_dismissed_since
 FROM capture_connection WHERE `+match.where+` FOR UPDATE`, match.args...).
		Scan(&view.ID, &view.Provider, &view.Status, &view.ConnectedAt, &view.SilenceDismissedSince); err != nil {
		return err
	}
	// Sync takes the connection lock before publishing delivery, so read the sidecar after it.
	err := tx.QueryRow(ctx, `SELECT last_delivery_at, last_success_at, last_error_class FROM capture_sync_state WHERE connection_id = @id`, pgx.NamedArgs{"id": view.ID}).
		Scan(&view.LastDeliveryAt, &view.LastSuccessAt, &view.LastErrorClass)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if silenceDismissed(view) && view.SilenceDismissedSince.Equal(since) {
		return nil
	}
	if !deliverySince(view).Equal(since) {
		return apperrors.ErrConflict
	}
	if silenceDismissed(view) {
		return nil
	}
	backfill, err := latestBackfill(ctx, tx, view.ID)
	if err != nil {
		return err
	}
	view.Backfill = backfill
	now := r.now()
	if connectionConcern(view, now) != ConcernCaptureSilent {
		return apperrors.ErrConflict
	}
	if _, err := tx.Exec(ctx, `UPDATE capture_connection SET silence_dismissed_since = @dismissed WHERE id = @id`, pgx.NamedArgs{"dismissed": since, "id": view.ID}); err != nil {
		return err
	}
	return auditLifecycle(ctx, tx, "update", captureConnectionObject, view.ID,
		map[string]any{auditKeyProvider: provider, "silence_dismissed_since": view.SilenceDismissedSince}, map[string]any{auditKeyProvider: provider, "silence_dismissed_since": since})
}
