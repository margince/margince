// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// Renewal of a Zalo OA's access token. The access token lives 25 hours and the
// refresh token is single use, and every refresh answers a new pair. A renewal
// therefore holds the connection's row lock from the Zalo call until the new
// pair is committed. A second run finds the row locked and skips it.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/capture/zalo"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ZaloRenewWithin is how long before expiry a token is replaced. The sweep runs
// every half hour, so a failed attempt has many retries before the token dies.
const ZaloRenewWithin = 6 * time.Hour

// capturedByZalo attributes the renewal's audit rows.
const capturedByZalo = "connector:" + zalo.Provider

// ZaloRenewer renews the access tokens of every connected Official Account.
type ZaloRenewer struct {
	Pool   *pgxpool.Pool
	Vault  keyvault.Vault
	Client *zalo.Client
	Log    *slog.Logger
	// Now is the clock the expiry is judged by.
	Now func() time.Time
}

// RenewWorkspace renews every connection of one workspace whose access token
// expires within ZaloRenewWithin. One connection's failure does not stop the
// others; the failures are returned joined so the job is retried.
func (r *ZaloRenewer) RenewWorkspace(ctx context.Context, workspace ids.UUID) error {
	ctx = principal.WithActor(principal.WithWorkspaceID(ctx, workspace), principal.Principal{
		Type: principal.PrincipalConnector, ID: capturedByZalo,
	})
	var live []ids.UUID
	err := database.WithWorkspaceTx(ctx, r.Pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT id FROM channel_connection WHERE status = $2 AND archived_at IS NULL AND `+zaloRow,
			zalo.Provider, channelStatusConnected)
		if err != nil {
			return err
		}
		live, err = pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
		return err
	})
	if err != nil {
		return fmt.Errorf("capture: listing the zalo connections to renew: %w", err)
	}
	var errs error
	for _, id := range live {
		if err := r.renewOne(ctx, workspace, id); err != nil {
			errs = errors.Join(errs, fmt.Errorf("capture: renewing zalo connection %s: %w", id, err))
		}
	}
	return errs
}

// zaloRenewal is what one locked renewal decided, applied after its
// transaction ends.
type zaloRenewal struct {
	oldRef, newRef keyvault.Ref
	reauth         bool
}

func (r *ZaloRenewer) renewOne(ctx context.Context, workspace, id ids.UUID) error {
	var done zaloRenewal
	err := database.WithWorkspaceTx(ctx, r.Pool, func(tx pgx.Tx) error {
		var err error
		done, err = r.renewLocked(ctx, tx, workspace, id)
		return err
	})
	if err != nil {
		// The new pair was sealed but the row never named it, so nothing can
		// read it. The old refresh token is spent, and the next run asks Zalo
		// to refuse it, which parks the connection for a new authorization.
		keyvault.DeleteDetached(ctx, r.Vault, r.Log, workspace, done.newRef, "zalo-renew-uncommitted")
		return err
	}
	keyvault.DeleteDetached(ctx, r.Vault, r.Log, workspace, done.oldRef, "zalo-token-renewed")
	if done.reauth {
		r.Log.WarnContext(ctx, "zalo: the refresh token was refused, so the connection needs a new authorization", "connection", id.String())
		return StopPollingChannelConnection(ctx, r.Pool, id, channelStatusReauthRequired, "zalo refused the refresh token")
	}
	return nil
}

// renewLocked runs inside the transaction that holds the connection's row lock.
// A row another run holds is skipped, and so is one whose token is not due.
func (r *ZaloRenewer) renewLocked(ctx context.Context, tx pgx.Tx, workspace, id ids.UUID) (zaloRenewal, error) {
	var appID, tokenRef, eventsRef string
	err := tx.QueryRow(ctx, `
		SELECT channel_id, credential_ref, coalesce(events_credential_ref, '') FROM channel_connection
		 WHERE id = $2 AND status = $3 AND archived_at IS NULL AND `+zaloRow+` FOR UPDATE SKIP LOCKED`,
		zalo.Provider, id, channelStatusConnected).Scan(&appID, &tokenRef, &eventsRef)
	if errors.Is(err, pgx.ErrNoRows) {
		return zaloRenewal{}, nil
	}
	if err != nil {
		return zaloRenewal{}, err
	}
	ws := ids.From[ids.WorkspaceKind](workspace)
	tokens, err := unsealZalo[zaloTokens](ctx, r.Vault, tx, ws, tokenRef)
	if err != nil {
		return zaloRenewal{}, err
	}
	secrets, err := unsealZalo[zaloSecrets](ctx, r.Vault, tx, ws, eventsRef)
	if err != nil {
		return zaloRenewal{}, err
	}
	if tokens.ExpiresAt.Sub(r.Now()) > ZaloRenewWithin {
		return zaloRenewal{}, nil
	}
	pair, err := r.Client.Refresh(ctx, zalo.App{ID: appID, Secret: secrets.AppSecret}, tokens.RefreshToken)
	if errors.Is(err, zalo.ErrReauthRequired) {
		return zaloRenewal{reauth: true}, nil
	}
	if err != nil {
		return zaloRenewal{}, err
	}
	//nolint:gosec // G117: the pair is sealed in the vault, never logged or returned
	sealed, err := json.Marshal(zaloTokens{AccessToken: pair.AccessToken, RefreshToken: pair.RefreshToken, ExpiresAt: r.Now().Add(pair.ExpiresIn).UTC()})
	if err != nil {
		return zaloRenewal{}, fmt.Errorf("encoding the renewed tokens: %w", err)
	}
	newRef, err := r.Vault.Put(ctx, ws, sealed)
	if err != nil {
		return zaloRenewal{}, fmt.Errorf("sealing the renewed tokens: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE channel_connection SET credential_ref = $2 WHERE id = $1`, id, string(newRef)); err != nil {
		return zaloRenewal{newRef: newRef}, err
	}
	return zaloRenewal{oldRef: keyvault.Ref(tokenRef), newRef: newRef}, nil
}

// unsealZalo reads one sealed JSON value through the open transaction, so the
// read needs no second connection while the row lock is held.
func unsealZalo[T any](ctx context.Context, vault keyvault.Vault, tx pgx.Tx, ws ids.WorkspaceID, ref string) (T, error) {
	var out T
	sealed, err := vault.GetOn(ctx, tx, ws, keyvault.Ref(ref))
	if err == nil {
		err = json.Unmarshal(sealed, &out)
	}
	if err != nil {
		return out, fmt.Errorf("unsealing the connection's credentials: %w", err)
	}
	return out, nil
}
