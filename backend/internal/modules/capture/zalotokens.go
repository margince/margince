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
	// Tx runs one renewal's locked work in a workspace transaction. Nil is
	// database.WithWorkspaceTx. It is the boundary a test breaks to make a
	// commit's outcome unclear.
	Tx func(ctx context.Context, fn func(pgx.Tx) error) error
}

func (r *ZaloRenewer) inTx(ctx context.Context, fn func(pgx.Tx) error) error {
	if r.Tx != nil {
		return r.Tx(ctx, fn)
	}
	return database.WithWorkspaceTx(ctx, r.Pool, fn)
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
	// refused says Zalo refused the refresh token.
	refused bool
	// spent says Zalo accepted the refresh token and issued a new pair, so the
	// old pair no longer works whatever else happened.
	spent bool
}

func (r *ZaloRenewer) renewOne(ctx context.Context, workspace, id ids.UUID) error {
	var done zaloRenewal
	err := r.inTx(ctx, func(tx pgx.Tx) error {
		var err error
		done, err = r.renewLocked(ctx, tx, workspace, id)
		return err
	})
	if done.refused {
		r.Log.WarnContext(ctx, "zalo: the refresh token was refused, so the connection needs a new authorization", "connection", id.String())
		return StopPollingChannelConnection(ctx, r.Pool, id, channelStatusReauthRequired, "zalo refused the refresh token")
	}
	if !done.spent {
		return err
	}
	return r.settle(ctx, workspace, id, done, err)
}

// settle decides what a spent refresh token left behind. The transaction's own
// answer cannot be trusted, because Postgres may have committed before the
// connection failed. So the row is read again, and a sealed credential is
// destroyed only when that read shows the row does not name it. When the read
// fails nothing is destroyed: an orphaned blob is inert, a destroyed live one is
// not.
func (r *ZaloRenewer) settle(ctx context.Context, workspace, id ids.UUID, done zaloRenewal, txErr error) error {
	current, err := r.credentialRef(ctx, id)
	if err != nil {
		r.Log.ErrorContext(ctx, "zalo: a renewal's outcome could not be read, so no credential was destroyed; the superseded or unnamed blob needs cleanup",
			"connection", id.String(), "old_credential_ref", string(done.oldRef), "new_credential_ref", string(done.newRef), "err", err)
		return errors.Join(txErr, err)
	}
	if done.newRef != "" && current == done.newRef {
		if txErr != nil {
			r.Log.WarnContext(ctx, "zalo: a renewal's commit reported an error but the connection names the new tokens", "connection", id.String(), "err", txErr)
		}
		keyvault.DeleteDetached(ctx, r.Vault, r.Log, workspace, done.oldRef, "zalo-token-renewed")
		return nil
	}
	keyvault.DeleteDetached(ctx, r.Vault, r.Log, workspace, done.newRef, "zalo-renew-unnamed")
	r.Log.ErrorContext(ctx, "zalo: Zalo issued new tokens that could not be stored, so the connection needs a new authorization", "connection", id.String(), "err", txErr)
	return errors.Join(txErr, StopPollingChannelConnection(ctx, r.Pool, id, channelStatusReauthRequired, "the renewed tokens could not be stored"))
}

// credentialRef reads which sealed credential the connection names now.
func (r *ZaloRenewer) credentialRef(ctx context.Context, id ids.UUID) (keyvault.Ref, error) {
	var ref string
	err := database.WithWorkspaceTx(ctx, r.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT credential_ref FROM channel_connection WHERE id = $1`, id).Scan(&ref)
	})
	return keyvault.Ref(ref), err
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
		return zaloRenewal{refused: true}, nil
	}
	if err != nil {
		return zaloRenewal{}, err
	}
	// From here Zalo has spent the old refresh token.
	//nolint:gosec // G117: the pair is sealed in the vault, never logged or returned
	sealed, err := json.Marshal(zaloTokens{AccessToken: pair.AccessToken, RefreshToken: pair.RefreshToken, ExpiresAt: r.Now().Add(pair.ExpiresIn).UTC()})
	if err != nil {
		return zaloRenewal{spent: true}, fmt.Errorf("encoding the renewed tokens: %w", err)
	}
	newRef, err := r.Vault.Put(ctx, ws, sealed)
	if err != nil {
		return zaloRenewal{spent: true}, fmt.Errorf("sealing the renewed tokens: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE channel_connection SET credential_ref = $2 WHERE id = $1`, id, string(newRef)); err != nil {
		return zaloRenewal{newRef: newRef, spent: true}, err
	}
	return zaloRenewal{oldRef: keyvault.Ref(tokenRef), newRef: newRef, spent: true}, nil
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
