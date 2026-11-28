// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// The steps every company chat account shares, whatever the network. They
// connect it with a reader team's seat, list it, change who reads it, disconnect
// it and read the keys its webhook is checked by. A network states its own
// columns, secrets and audit image, and these functions run the write shape.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// chatAccountRow selects the connections of one network that name their account.
const chatAccountRow = "provider = $1 AND account_id IS NOT NULL"

// chatAccount is one account to connect. Token and Events are the values sealed
// behind credential_ref and events_credential_ref.
type chatAccount struct {
	Provider, Account, Channel, Label string
	ReaderTeam                        ids.UUID
	Token, Events                     string
	// Required names each pasted value the admin must have filled in.
	Required map[string]string
	// Noun is the account in the admin's words, such as "phone number".
	Noun string
}

// connectChatAccount seals the account's secrets and writes its live row, its
// team seat and its audit row in one transaction. An account connects once: the
// unique index on (provider, account_id) refuses a second live row. load reads
// the new row back in that transaction and answers its audit image.
func (s *ChannelStore) connectChatAccount(ctx context.Context, a chatAccount, load func(tx pgx.Tx, id ids.UUID) (map[string]any, error)) error {
	if err := auth.Require(ctx, channelConnectionObject, principal.ActionCreate); err != nil {
		return err
	}
	if err := auth.RequireHuman(ctx); err != nil {
		return err
	}
	if err := a.validate(); err != nil {
		return err
	}
	if s.vault == nil {
		return fmt.Errorf("configure a credential store for this installation, so the secrets can be sealed: %w", ErrChannelWiringIncomplete)
	}
	company, err := s.db.Workspace(ctx)
	if err != nil {
		return fmt.Errorf("capture: %s connect needs a live company: %w", a.Provider, err)
	}
	ws := company.UUID
	connectedBy, err := channelActor(ctx)
	if err != nil {
		return err
	}
	refs, err := s.sealConnectionSecrets(ctx, ws, a.Token, a.Events)
	if err != nil {
		return err
	}
	err = s.db.Tx(ctx, func(tx pgx.Tx) error { return insertChatAccount(ctx, tx, a, refs, connectedBy, load) })
	if err == nil {
		return nil
	}
	// A lost race for the unique index proves no row persisted, so the refs are
	// safe to destroy. Any other error leaves the commit outcome open.
	if _, unique := storekit.UniqueViolation(err); unique {
		s.destroyDetached(ctx, ws, a.Provider+"-connect-lost-race", refs.token, refs.events)
		return fmt.Errorf("this %s is already connected; disconnect it first: %w", a.Noun, apperrors.ErrConflict)
	}
	var teamErr *ReaderTeamError
	var reconnectErr *SharedReconnectError
	if errors.As(err, &teamErr) || errors.As(err, &reconnectErr) {
		// The transaction rolled back, so no row names the refs.
		s.destroyDetached(ctx, ws, a.Provider+"-connect-refused", refs.token, refs.events)
		return err
	}
	return fmt.Errorf("capture: connecting a %s: %w", a.Provider, err)
}

func (a chatAccount) validate() error {
	if a.ReaderTeam.IsZero() {
		return &ReaderTeamError{missing: true}
	}
	for what, value := range a.Required {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("enter %s: %w", what, apperrors.ErrInvalidArgument)
		}
	}
	return nil
}

func insertChatAccount(ctx context.Context, tx pgx.Tx, a chatAccount, refs connectionRefs, connectedBy ids.UUID,
	load func(tx pgx.Tx, id ids.UUID) (map[string]any, error),
) error {
	seat, err := EnsureChatSeatTx(ctx, tx, ChatSeat{Provider: a.Provider, Account: a.Account, ReaderTeam: a.ReaderTeam, By: connectedBy})
	if err != nil {
		return err
	}
	var id ids.UUID
	if err := tx.QueryRow(ctx, `
		INSERT INTO channel_connection
		  (provider, channel_id, channel_label, account_id, credential_ref, events_credential_ref,
		   status, connected_by, approved_at, seat_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, now(), $9)
		RETURNING id`,
		a.Provider, a.Channel, a.Label, a.Account, string(refs.token), string(refs.events),
		channelStatusConnected, connectedBy, seat).Scan(&id); err != nil {
		return err
	}
	image, err := load(tx, id)
	if err != nil {
		return err
	}
	return auditLifecycle(ctx, tx, "create", channelConnectionObject, id, nil, image)
}

// listChatAccounts returns one network's live accounts, newest first. columns is
// that network's own select list, scanned by position into T.
func listChatAccounts[T any](ctx context.Context, s *ChannelStore, provider, columns string) ([]T, error) {
	if err := requireBusinessAdmin(ctx, principal.ActionUpdate); err != nil {
		return nil, err
	}
	var out []T
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT `+columns+` FROM channel_connection
			 WHERE archived_at IS NULL AND `+chatAccountRow+` ORDER BY created_at DESC, id DESC`, provider)
		if err != nil {
			return err
		}
		out, err = pgx.CollectRows(rows, pgx.RowToStructByPos[T])
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("capture: listing %s connections: %w", provider, err)
	}
	return out, nil
}

// disconnectChatAccount archives the account, ends its seat and destroys its
// sealed secrets. From the archive on the webhook route stores nothing for it.
// What it filed stays. image answers the account's audit image in the transaction.
func (s *ChannelStore) disconnectChatAccount(ctx context.Context, provider string, id ids.UUID,
	image func(tx pgx.Tx, id ids.UUID) (map[string]any, error),
) error {
	if err := requireBusinessAdmin(ctx, principal.ActionDelete); err != nil {
		return err
	}
	if s.vault == nil {
		return fmt.Errorf("configure a credential store for this installation, so the sealed secrets can be destroyed: %w", ErrChannelWiringIncomplete)
	}
	company, err := s.db.Workspace(ctx)
	if err != nil {
		return fmt.Errorf("capture: %s disconnect needs a live company: %w", provider, err)
	}
	ws := company.UUID
	var refs connectionRefs
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		var seat *ids.UUID
		var tokenRef, eventsRef string
		err := tx.QueryRow(ctx, `
			SELECT seat_id, credential_ref, coalesce(events_credential_ref, '') FROM channel_connection
			 WHERE id = $2 AND archived_at IS NULL AND `+chatAccountRow+` FOR UPDATE`, provider, id).
			Scan(&seat, &tokenRef, &eventsRef)
		if errors.Is(err, pgx.ErrNoRows) {
			return apperrors.ErrNotFound
		}
		if err != nil {
			return err
		}
		before, err := image(tx, id)
		if err != nil {
			return err
		}
		after := make(map[string]any, len(before))
		maps.Copy(after, before)
		after[columnStatus] = channelStatusDisconnected
		if _, err := tx.Exec(ctx, `UPDATE channel_connection SET status = $2, archived_at = now() WHERE id = $1`,
			id, channelStatusDisconnected); err != nil {
			return err
		}
		refs = connectionRefs{token: keyvault.Ref(tokenRef), events: keyvault.Ref(eventsRef)}
		if seat != nil {
			if err := EndChatSeatTx(ctx, tx, *seat); err != nil {
				return err
			}
		}
		return auditLifecycle(ctx, tx, "archive", channelConnectionObject, id, before, after)
	})
	if errors.Is(err, apperrors.ErrNotFound) {
		return err
	}
	if err != nil {
		return fmt.Errorf("capture: disconnecting a %s connection: %w", provider, err)
	}
	s.destroyDetached(ctx, ws, provider+"-disconnected", refs.token, refs.events)
	return nil
}

// changeChatAccountVisibility sets who may read an account's messages, by the
// posture change a shared mailbox's PATCH makes on the account's seat. The
// visibility is held or shared, `shared` needs the workspace opt-in, and the
// admin confirms. read loads the changed connection in the same transaction.
func (s *ChannelStore) changeChatAccountVisibility(ctx context.Context, provider string, id ids.UUID, posture string, confirmed bool,
	read func(tx pgx.Tx) error,
) error {
	if err := requireBusinessAdmin(ctx, principal.ActionUpdate); err != nil {
		return err
	}
	if !confirmed {
		return &SharedChangeError{}
	}
	if err := refuseSharedVisibility(posture); err != nil {
		return err
	}
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		var seat *ids.UUID
		if err := tx.QueryRow(ctx, `SELECT seat_id FROM channel_connection
			 WHERE id = $2 AND archived_at IS NULL AND `+chatAccountRow+` FOR UPDATE`, provider, id).Scan(&seat); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return apperrors.ErrNotFound
			}
			return err
		}
		if err := changeSeatPostureTx(ctx, tx, seat, posture); err != nil {
			return err
		}
		return read(tx)
	})
	if errors.Is(err, apperrors.ErrNotFound) || errors.Is(err, apperrors.ErrConflict) {
		return err
	}
	if err != nil {
		return fmt.Errorf("capture: changing the visibility of a %s connection: %w", provider, err)
	}
	return nil
}

// liveChatEndpoint is one live account with the secrets its webhook is checked
// with. S is the network's own sealed value.
type liveChatEndpoint[S any] struct {
	ConnectionID ids.UUID
	Account      string
	Channel      string
	Secrets      S
}

// liveChatEndpoints returns every account of provider that still receives, with
// its unsealed secrets. `reauth_required` still receives: a signed message
// authenticates without the access token. An account whose secrets cannot be read
// is left out and logged by id, so one broken seal does not stop the others.
func liveChatEndpoints[S any](ctx context.Context, db *database.DB, vault func() keyvault.Vault, log *slog.Logger, provider string,
) ([]liveChatEndpoint[S], error) {
	v := vault()
	company, err := db.Workspace(ctx)
	if v == nil || err != nil {
		return nil, ErrChannelWiringIncomplete
	}
	ws := company.UUID
	type row struct {
		id                  ids.UUID
		account, channel, s string
	}
	var rows []row
	err = db.Tx(ctx, func(tx pgx.Tx) error {
		res, err := tx.Query(ctx, `SELECT id, account_id, channel_id, events_credential_ref FROM channel_connection
			 WHERE archived_at IS NULL AND status IN ($2, $3) AND events_credential_ref IS NOT NULL AND `+chatAccountRow+`
			 ORDER BY created_at, id`, provider, channelStatusConnected, channelStatusReauthRequired)
		if err != nil {
			return err
		}
		defer res.Close()
		for res.Next() {
			var r row
			if err := res.Scan(&r.id, &r.account, &r.channel, &r.s); err != nil {
				return err
			}
			rows = append(rows, r)
		}
		return res.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("capture: reading the live %s accounts: %w", provider, err)
	}
	out := make([]liveChatEndpoint[S], 0, len(rows))
	for _, r := range rows {
		sealed, err := v.Get(ctx, ids.From[ids.WorkspaceKind](ws), keyvault.Ref(r.s))
		var secrets S
		if err == nil {
			err = json.Unmarshal(sealed, &secrets)
		}
		if err != nil {
			log.WarnContext(ctx, provider+": the sealed webhook secrets of a connection cannot be read", "connection", r.id.String(), "err", err)
			continue
		}
		out = append(out, liveChatEndpoint[S]{ConnectionID: r.id, Account: r.account, Channel: r.channel, Secrets: secrets})
	}
	return out, nil
}
