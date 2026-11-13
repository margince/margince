// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/settings"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/workforce"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// InternalMailSourceOption names a live connection without resolving its credential.
// A shared mailbox has no owner, and its address is its name.
type InternalMailSourceOption struct {
	ConnectionID ids.UUID
	OwnerUserID  *ids.UUID
	Provider     string
	AccountLabel string
	OwnerName    string
	Generation   int
}

// ListInternalMailSources offers mail connections their owners can still access,
// and shared mailboxes whose reader team is live.
func (s *SettingsStore) ListInternalMailSources(ctx context.Context) ([]InternalMailSourceOption, error) {
	if err := auth.RequireAdmin(ctx); err != nil {
		return nil, err
	}
	if err := settings.RequireRead(ctx, internalMailPolicy); err != nil {
		return nil, err
	}
	out := []InternalMailSourceOption{}
	err := s.settings.WriteTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT c.id, c.user_id, c.provider, coalesce(c.account_label, ''), coalesce(u.display_name, ''), c.generation
   FROM capture_connection c LEFT JOIN app_user u ON u.id = c.user_id
   WHERE c.provider IN ('gmail','graph','imap') AND c.status IN ('connected','error')
    AND c.archived_at IS NULL AND ((u.id IS NOT NULL AND `+workforce.LiveHumanSQL("u")+`) OR `+sharedMailboxLiveSQL+`)
   ORDER BY u.display_name NULLS FIRST, c.account_label, c.provider, c.id`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var source InternalMailSourceOption
			if err := rows.Scan(&source.ConnectionID, &source.OwnerUserID, &source.Provider, &source.AccountLabel, &source.OwnerName, &source.Generation); err != nil {
				return err
			}
			out = append(out, source)
		}
		return rows.Err()
	})
	return out, err
}

// InternalMailSourceContainers reuses the provider listing under administrator authorization.
func (r *Registry) InternalMailSourceContainers(ctx context.Context, connection ids.UUID, provider string) ([]connector.NamedContainer, bool, error) {
	actor, ok := principal.Actor(ctx)
	if !ok || actor.Type != principal.PrincipalHuman {
		return nil, false, apperrors.ErrPermissionDenied
	}
	if err := settings.RequireRead(ctx, internalMailPolicy); err != nil {
		return nil, false, err
	}
	var source InternalMailSource
	err := r.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		source, err = readInternalMailSource(ctx, tx, connection)
		return err
	})
	if err != nil {
		return nil, false, err
	}
	if source.Provider != provider {
		return nil, false, apperrors.ErrNotFound
	}
	if source.OwnerUserID == nil {
		return r.listContainersOf(ctx, source.Provider, `id = $1 AND user_id IS NULL`, source.ConnectionID)
	}
	return r.ListContainers(ctx, source.Provider, ids.From[ids.UserKind](*source.OwnerUserID))
}
