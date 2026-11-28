// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatlinks

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/platform/teamchat"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// MessengerProviders exposes workspace service choices without exposing personal grants.
func (s *Store) MessengerProviders(ctx context.Context) ([]string, error) {
	if _, err := memberActor(ctx); err != nil {
		return nil, err
	}
	var providers []string
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		if _, err := s.readMemberTx(ctx, tx); err != nil {
			return err
		}
		var err error
		providers, err = teamchat.Providers(ctx, tx)
		return err
	})
	return providers, err
}

// DisconnectProviderMembers withdraws every local personal grant for an administrator's service switch.
func (s *Store) DisconnectProviderMembers(ctx context.Context, provider string, expected teamchat.Selection) error {
	if _, err := memberMutationActor(ctx); err != nil {
		return err
	}
	if !validProvider(provider) {
		return apperrors.ErrInvalidArgument
	}
	workspace, err := s.db.Workspace(ctx)
	if err != nil {
		return err
	}
	var retired []keyvault.Ref
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		ctx, err := s.holdMemberTx(ctx, tx)
		if err != nil {
			return err
		}
		if err := auth.Require(ctx, "channel_connection", principal.ActionDelete); err != nil {
			return err
		}
		if err := teamchat.HoldSelection(ctx, tx, expected); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, "SELECT "+memberGrantColumns+" FROM chat_member_grant WHERE provider=@provider AND status<>'disconnected' ORDER BY id FOR UPDATE", pgx.StrictNamedArgs{providerKey: provider})
		if err != nil {
			return err
		}
		grants, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (MemberGrant, error) { return scanMemberGrant(row) })
		if err != nil {
			return err
		}
		for _, before := range grants {
			if err := disconnectMemberGrantTx(ctx, tx, before, "service_switch"); err != nil {
				return err
			}
			retired = append(retired, keyvault.Ref(before.credential))
		}
		return nil
	})
	if err == nil && s.vault != nil {
		for _, ref := range retired {
			keyvault.DeleteDetached(ctx, s.vault(), slog.Default(), workspace.UUID, ref, "team messaging service disconnect")
		}
	}
	return err
}

// SelectMessengerProvider pins the choice before old credentials are withdrawn.
func (s *Store) SelectMessengerProvider(ctx context.Context, provider string, expected ...teamchat.Selection) (teamchat.Selection, error) {
	if _, err := memberMutationActor(ctx); err != nil {
		return teamchat.Selection{}, err
	}
	if provider != "" && !validProvider(provider) {
		return teamchat.Selection{}, apperrors.ErrInvalidArgument
	}
	var selected teamchat.Selection
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		ctx, err := s.holdMemberTx(ctx, tx)
		if err != nil {
			return err
		}
		if err := auth.Require(ctx, "channel_connection", principal.ActionDelete); err != nil {
			return err
		}
		if err := teamchat.Lock(ctx, tx); err != nil {
			return err
		}
		var id ids.UUID
		before, err := teamchat.ReadSelection(ctx, tx)
		if err != nil {
			return err
		}
		if len(expected) > 0 && before != expected[0] {
			return apperrors.ErrVersionSkew
		}
		action := updateKey
		if before.Version == 0 {
			action = "create"
			id = ids.NewV7()
			if _, err := tx.Exec(ctx, "INSERT INTO chat_service(id) VALUES(@id)", pgx.StrictNamedArgs{"id": id}); err != nil {
				return err
			}
		} else if err := tx.QueryRow(ctx, "SELECT id FROM chat_service WHERE singleton").Scan(&id); err != nil {
			return err
		}
		selected = teamchat.Selection{Provider: provider, Version: before.Version + 1}
		if _, err := tx.Exec(ctx, "UPDATE chat_service SET provider=nullif(@provider,''),version=@version,updated_at=now() WHERE singleton", pgx.StrictNamedArgs{providerKey: provider, versionKey: selected.Version}); err != nil {
			return err
		}
		audit, err := storekit.Audit(ctx, tx, action, "chat_service", id, before, selected)
		if err != nil {
			return err
		}
		return storekit.EmitEvent(ctx, tx, audit, id, crmcontracts.InternalEventChatServiceSelected{})
	})
	return selected, err
}

// SelectedMessengerProvider is configuration, independent of whether any account has connected yet.
func (s *Store) SelectedMessengerProvider(ctx context.Context) (string, error) {
	selected, err := s.MessengerSelection(ctx)
	return selected.Provider, err
}

// MessengerSelection binds service cleanup to the choice the caller read.
func (s *Store) MessengerSelection(ctx context.Context) (teamchat.Selection, error) {
	if _, err := memberActor(ctx); err != nil {
		return teamchat.Selection{}, err
	}
	var selected teamchat.Selection
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		if _, err := s.readMemberTx(ctx, tx); err != nil {
			return err
		}
		var err error
		selected, err = teamchat.ReadSelection(ctx, tx)
		return err
	})
	return selected, err
}

// EndProviderLinks frees records for the selected service while retaining ended-sync history.
func (s *Store) EndProviderLinks(ctx context.Context, provider string, expected teamchat.Selection) error {
	if _, err := memberMutationActor(ctx); err != nil {
		return err
	}
	if !validProvider(provider) {
		return apperrors.ErrInvalidArgument
	}
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		ctx, err := s.holdMemberTx(ctx, tx)
		if err != nil {
			return err
		}
		if err := auth.Require(ctx, "channel_connection", principal.ActionDelete); err != nil {
			return err
		}
		if err := teamchat.HoldSelection(ctx, tx, expected); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, "SELECT "+linkColumns+" FROM chat_link WHERE provider=@provider AND archived_at IS NULL ORDER BY id FOR UPDATE", pgx.StrictNamedArgs{providerKey: provider})
		if err != nil {
			return err
		}
		links, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Link, error) { return scanLink(row) })
		if err != nil {
			return err
		}
		for _, link := range links {
			if err := retireLink(ctx, tx, link); err != nil {
				return err
			}
		}
		return nil
	})
}
