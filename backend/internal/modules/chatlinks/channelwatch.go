// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatlinks

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/chatlinks/teams"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ChannelWatch pins a notification lease to its installation and channel.
type ChannelWatch struct {
	Retired         *time.Time
	RemovalVerified bool
	ID              ids.UUID
	Connection      ids.UUID
	Channel         string
	App             string
	Tenant          string
	URL             string
	Subscription    string
	Expires         *time.Time
	LeaseUntil      *time.Time
	Reauthorize     bool
	Rescan          bool
	Version         int64
	Error           *string
	credential      string
}

const watchColumns = `id,connection_id,channel_id,app_id,tenant_id,notification_url,coalesce(subscription_id,''),expires_at,reauthorize,needs_rescan,version,last_error,coalesce(credential_ref,''),retired_at,removal_verified,lease_until`

func scanChannelWatch(row pgx.Row) (ChannelWatch, error) {
	var out ChannelWatch
	err := row.Scan(&out.ID, &out.Connection, &out.Channel, &out.App, &out.Tenant, &out.URL, &out.Subscription, &out.Expires, &out.Reauthorize, &out.Rescan, &out.Version, &out.Error, &out.credential, &out.Retired, &out.RemovalVerified, &out.LeaseUntil)
	return out, err
}

// PrepareChannelWatch pins a watch to the link's verified installation identity.
func (s *Store) PrepareChannelWatch(ctx context.Context, link Link, app, tenant, address string, now time.Time) (ChannelWatch, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return ChannelWatch{}, err
	}
	if s.vault == nil || s.vault() == nil {
		return ChannelWatch{}, ErrEventsUnavailable
	}
	if !validChannelWatchTarget(link, app, tenant, address, now) {
		return ChannelWatch{}, apperrors.ErrInvalidArgument
	}
	candidate := ChannelWatch{ID: ids.NewV7(), Connection: link.ConnectionID, Channel: link.ChannelSlug, App: app, Tenant: tenant, URL: address, Version: 1}
	var out ChannelWatch
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := s.HoldLiveLinkTx(ctx, tx, link); err != nil {
			return err
		}
		var err error
		out, err = readChannelWatch(ctx, tx, candidate)
		return err
	})
	if err == nil && out.Retired == nil {
		return matchChannelWatch(out, candidate)
	}
	if err == nil {
		err = pgx.ErrNoRows
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return ChannelWatch{}, err
	}
	workspace, err := s.db.Workspace(ctx)
	if err != nil {
		return ChannelWatch{}, err
	}
	ref, err := s.sealWatchCredential(ctx)
	if err != nil {
		return ChannelWatch{}, err
	}
	candidate.credential = string(ref)
	out, err = s.insertChannelWatch(ctx, link, candidate, now)
	if err == nil && out.credential != candidate.credential || watchWriteRolledBack(err) {
		keyvault.DeleteDetached(ctx, s.vault(), slog.Default(), workspace.UUID, ref, "unused channel watch credential")
	}
	return out, err
}

func readChannelWatch(ctx context.Context, tx pgx.Tx, watch ChannelWatch) (ChannelWatch, error) {
	return scanChannelWatch(tx.QueryRow(ctx, `SELECT `+watchColumns+` FROM chat_channel_watch WHERE connection_id=@connection AND channel_id=@channel`, pgx.StrictNamedArgs{connectionKey: watch.Connection, channelKey: watch.Channel}))
}

func matchChannelWatch(current, candidate ChannelWatch) (ChannelWatch, error) {
	if current.App != candidate.App || current.Tenant != candidate.Tenant || current.URL != candidate.URL || current.credential == "" {
		return current, apperrors.ErrConflict
	}
	return current, nil
}

func (s *Store) insertChannelWatch(ctx context.Context, link Link, candidate ChannelWatch, now time.Time) (ChannelWatch, error) {
	var out ChannelWatch
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := s.HoldLiveLinkTx(ctx, tx, link); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended(@key,0))`, pgx.StrictNamedArgs{keyKey: "chat-watch:" + link.ConnectionID.String() + ":" + link.ChannelSlug}); err != nil {
			return err
		}
		var err error
		out, err = readChannelWatch(ctx, tx, candidate)
		if err == nil && out.Retired != nil {
			before := out
			out = candidate
			out.ID = before.ID
			out.Version = before.Version + 1
			return reactivateChannelWatch(ctx, tx, before, out)
		}
		if err == nil {
			out, err = matchChannelWatch(out, candidate)
			return err
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		out = candidate
		if _, err := tx.Exec(ctx, `INSERT INTO chat_channel_watch(id,connection_id,channel_id,app_id,tenant_id,notification_url,credential_ref,created_at) VALUES(@id,@connection,@channel,@app,@tenant,@url,@credential,@created)`, pgx.StrictNamedArgs{"id": out.ID, "connection": out.Connection, channelKey: out.Channel, appKey: out.App, tenantKey: out.Tenant, urlKey: out.URL, "credential": out.credential, "created": now}); err != nil {
			return err
		}
		_, err = storekit.Audit(ctx, tx, "create", "chat_channel_watch", out.ID, nil, channelWatchImage(out))
		return err
	})
	return out, err
}

// ChannelWatchSecret opens the secret belonging to the exact stored watch binding.
func (s *Store) ChannelWatchSecret(ctx context.Context, watch ChannelWatch) (string, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return "", err
	}
	if s.vault == nil || s.vault() == nil || watch.credential == "" {
		return "", ErrEventsUnavailable
	}
	workspace, err := s.db.Workspace(ctx)
	if err != nil {
		return "", err
	}
	secret, err := s.vault().Get(ctx, workspace, keyvault.Ref(watch.credential))
	return string(secret), err
}

func channelWatchImage(watch ChannelWatch) map[string]any {
	return map[string]any{"notification_url": watch.URL, "retired_at": watch.Retired, "removal_verified": watch.RemovalVerified, "connection_id": watch.Connection, channelIDKey: watch.Channel, "app_id": watch.App, "tenant_id": watch.Tenant, "subscription_id": watch.Subscription, "expires_at": watch.Expires, "lease_until": watch.LeaseUntil, "reauthorize": watch.Reauthorize, "needs_rescan": watch.Rescan, versionKey: watch.Version, "last_error": watch.Error, "credential_set": watch.credential != ""}
}

func (s *Store) sealWatchCredential(ctx context.Context) (keyvault.Ref, error) {
	workspace, err := s.db.Workspace(ctx)
	if err != nil {
		return "", err
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return "", err
	}
	return s.vault().Put(ctx, workspace, []byte(base64.RawURLEncoding.EncodeToString(secret)))
}

func watchWriteRolledBack(err error) bool {
	return errors.Is(err, apperrors.ErrConflict) || errors.Is(err, apperrors.ErrNotFound) || errors.Is(err, apperrors.ErrPermissionDenied) || errors.Is(err, ErrLinkNotFound) || errors.Is(err, pgx.ErrNoRows)
}

func validChannelWatchTarget(link Link, app, tenant, address string, now time.Time) bool {
	return !now.IsZero() && link.Provider == ProviderTeams && !link.ReadDisabled && app != "" && tenant != "" && teams.ValidWatchURL(address)
}
