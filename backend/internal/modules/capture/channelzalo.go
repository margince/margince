// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// Zalo Official Account connections, on the steps in chataccount.go. The row is
// `connected` from the start, and a reader team's seat takes the OA's messages.
// account_id is the OA id, channel_id the Zalo app id and channel_label the
// display name. The token pair is sealed behind credential_ref and replaced on
// every renewal. The app secret and the OA secret key are sealed behind
// events_credential_ref.

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/capture/zalo"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// zaloRow selects the Zalo connections, each of which names its OA. The provider
// is bound as $1 where this is used.
const zaloRow = chatAccountRow

const zaloColumns = `id, account_id, channel_id, channel_label, status, created_at, seat_id,
	(SELECT s.reader_team_id FROM capture_connection s WHERE s.id = channel_connection.seat_id),
	(SELECT s.mail_posture FROM capture_connection s WHERE s.id = channel_connection.seat_id)`

// ZaloOAConnection is one connected Official Account as an admin reads it.
type ZaloOAConnection struct {
	ID          ids.UUID
	OAID        string
	AppID       string
	DisplayName string
	Status      string
	ConnectedAt time.Time
	// SeatID is the seat the OA's messages file through. ReaderTeam reads them,
	// and Visibility is the seat's posture, held or shared.
	SeatID     *ids.UUID
	ReaderTeam *ids.UUID
	Visibility *string
}

// ConnectZaloOARequest is what the admin pastes from the Zalo developer console.
type ConnectZaloOARequest struct {
	OAID, AppID, DisplayName  string
	AppSecret, OASecretKey    string
	AccessToken, RefreshToken string
	// ReaderTeam reads the OA's messages. The OA's seat starts held.
	ReaderTeam ids.UUID
}

// zaloTokens is the value sealed behind credential_ref. ExpiresAt is when the
// access token stops working; the renewal reads it to decide whether to refresh.
type zaloTokens struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// zaloSecrets is the value sealed behind events_credential_ref.
type zaloSecrets struct {
	AppSecret   string `json:"app_secret"`
	OASecretKey string `json:"oa_secret_key"`
}

func scanZaloConnection(r pgx.Row) (ZaloOAConnection, error) {
	var c ZaloOAConnection
	err := r.Scan(&c.ID, &c.OAID, &c.AppID, &c.DisplayName, &c.Status, &c.ConnectedAt, &c.SeatID, &c.ReaderTeam, &c.Visibility)
	return c, err
}

func readZaloOA(ctx context.Context, tx pgx.Tx, id ids.UUID) (ZaloOAConnection, error) {
	return scanZaloConnection(tx.QueryRow(ctx, `SELECT `+zaloColumns+` FROM channel_connection WHERE id = $1`, id))
}

func zaloAuditImage(c ZaloOAConnection) map[string]any {
	return map[string]any{
		detailProvider: zalo.Provider,
		"oa_id":        c.OAID,
		"app_id":       c.AppID,
		"display_name": c.DisplayName,
		columnStatus:   c.Status,
		"seat_id":      c.SeatID,
	}
}

// ConnectZaloOA seals the OA's secrets and writes its live row, its seat and its
// audit row. The pasted access token is treated as already due, so the first
// renewal replaces the pair with one whose expiry Zalo stated.
func (s *ChannelStore) ConnectZaloOA(ctx context.Context, req ConnectZaloOARequest) (ZaloOAConnection, error) {
	//nolint:gosec // G117: the pair is sealed in the vault, never logged or returned
	tokens, err := json.Marshal(zaloTokens{AccessToken: req.AccessToken, RefreshToken: req.RefreshToken, ExpiresAt: time.Now().UTC()})
	if err != nil {
		return ZaloOAConnection{}, fmt.Errorf("capture: encoding the zalo tokens: %w", err)
	}
	secrets, err := json.Marshal(zaloSecrets{AppSecret: req.AppSecret, OASecretKey: req.OASecretKey})
	if err != nil {
		return ZaloOAConnection{}, fmt.Errorf("capture: encoding the zalo webhook secrets: %w", err)
	}
	var out ZaloOAConnection
	err = s.connectChatAccount(ctx, chatAccount{
		Provider: zalo.Provider, Account: req.OAID, Channel: req.AppID, Label: req.DisplayName,
		ReaderTeam: req.ReaderTeam, Token: string(tokens), Events: string(secrets), Noun: "Official Account",
		Required: map[string]string{
			"the Official Account id": req.OAID, "the app id": req.AppID, "the display name": req.DisplayName,
			"the app secret": req.AppSecret, "the OA secret key": req.OASecretKey,
			"the access token": req.AccessToken, "the refresh token": req.RefreshToken,
		},
	}, func(tx pgx.Tx, id ids.UUID) (map[string]any, error) {
		var err error
		out, err = readZaloOA(ctx, tx, id)
		return zaloAuditImage(out), err
	})
	return out, err
}

// ListZaloOAConnections returns the live Official Accounts, newest first.
func (s *ChannelStore) ListZaloOAConnections(ctx context.Context) ([]ZaloOAConnection, error) {
	return listChatAccounts[ZaloOAConnection](ctx, s, zalo.Provider, zaloColumns)
}

// DisconnectZaloOA archives the OA, ends its seat and destroys its sealed
// secrets. From the archive on the webhook route stores nothing for it.
func (s *ChannelStore) DisconnectZaloOA(ctx context.Context, id ids.UUID) error {
	return s.disconnectChatAccount(ctx, zalo.Provider, id, func(tx pgx.Tx, id ids.UUID) (map[string]any, error) {
		current, err := readZaloOA(ctx, tx, id)
		return zaloAuditImage(current), err
	})
}

// ChangeZaloOAVisibility sets who may read the messages of a connected OA, as
// ChangeWhatsAppVisibility does for a WhatsApp number.
func (s *ChannelStore) ChangeZaloOAVisibility(ctx context.Context, id ids.UUID, posture string, confirmed bool) (ZaloOAConnection, error) {
	var out ZaloOAConnection
	err := s.changeChatAccountVisibility(ctx, zalo.Provider, id, posture, confirmed, func(tx pgx.Tx) error {
		var err error
		out, err = readZaloOA(ctx, tx, id)
		return err
	})
	return out, err
}

// ZaloEndpoint is one live OA with the secret its webhook deliveries are signed
// with. It is what the unauthenticated webhook route reads, so it exists only
// for that route and never reaches a response body.
type ZaloEndpoint struct {
	ConnectionID ids.UUID
	OAID         string
	AppID        string
	OASecretKey  string
}

// ZaloEdge reads the live OAs' webhook secrets. It carries no principal gate
// because Zalo's call has none; the signature is what the caller checks against
// what this returns.
type ZaloEdge struct {
	db    *database.DB
	vault func() keyvault.Vault
	log   *slog.Logger
}

// NewZaloEdge reads through vault, which may be late-bound.
func NewZaloEdge(db *database.DB, vault func() keyvault.Vault, log *slog.Logger) *ZaloEdge {
	return &ZaloEdge{db: db, vault: vault, log: log}
}

// LiveEndpoints returns every OA that still receives, with its unsealed signing
// key. An OA that needs a new authorization still receives, because the
// signature does not depend on the access token.
func (e *ZaloEdge) LiveEndpoints(ctx context.Context) ([]ZaloEndpoint, error) {
	live, err := liveChatEndpoints[zaloSecrets](ctx, e.db, e.vault, e.log, zalo.Provider)
	if err != nil {
		return nil, err
	}
	out := make([]ZaloEndpoint, 0, len(live))
	for _, l := range live {
		out = append(out, ZaloEndpoint{ConnectionID: l.ConnectionID, OAID: l.Account, AppID: l.Channel, OASecretKey: l.Secrets.OASecretKey})
	}
	return out, nil
}
