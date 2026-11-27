// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// WhatsApp Business connections. An admin connects one phone number with the
// credentials of the Meta app that serves it. The admin is the approver, so the
// row is `connected` from the start.
//
// account_id is the phone number id, channel_id the WhatsApp Business Account
// id, channel_label the number in display form. The access token is sealed
// behind credential_ref. The app secret and the verify token share one sealed
// value behind events_credential_ref, because the webhook reads them together.

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/capture/whatsapp"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// whatsappRow selects the WhatsApp connections, each of which names its number.
const whatsappRow = "provider = '" + whatsapp.Provider + "' AND account_id IS NOT NULL"

const whatsappColumns = `id, channel_id, account_id, channel_label, status, created_at, seat_id,
	(SELECT s.reader_team_id FROM capture_connection s WHERE s.id = channel_connection.seat_id),
	(SELECT s.mail_posture FROM capture_connection s WHERE s.id = channel_connection.seat_id)`

// verifyTokenBytes is the size of the random verify token before encoding.
const verifyTokenBytes = 32

// WhatsAppConnection is one connected WhatsApp Business number as an admin reads it.
type WhatsAppConnection struct {
	ID            ids.UUID
	WABAID        string
	PhoneNumberID string
	DisplayPhone  string
	Status        string
	ConnectedAt   time.Time
	// SeatID is the seat the number's messages file through. ReaderTeam reads
	// them, and Visibility is the seat's posture, held or shared.
	SeatID     *ids.UUID
	ReaderTeam *ids.UUID
	Visibility *string
}

// ConnectWhatsAppRequest is what the admin pastes from Meta.
type ConnectWhatsAppRequest struct {
	WABAID, PhoneNumberID, DisplayPhone string
	AccessToken, AppSecret              string
	// ReaderTeam reads the number's messages. The number's seat starts held.
	ReaderTeam ids.UUID
}

// whatsappSecrets is the value sealed behind events_credential_ref.
type whatsappSecrets struct {
	AppSecret   string `json:"app_secret"`
	VerifyToken string `json:"verify_token"`
}

func scanWhatsAppConnection(r pgx.Row) (WhatsAppConnection, error) {
	var c WhatsAppConnection
	err := r.Scan(&c.ID, &c.WABAID, &c.PhoneNumberID, &c.DisplayPhone, &c.Status, &c.ConnectedAt, &c.SeatID, &c.ReaderTeam, &c.Visibility)
	return c, err
}

func whatsappAuditImage(c WhatsAppConnection) map[string]any {
	return map[string]any{
		detailProvider:    whatsapp.Provider,
		"waba_id":         c.WABAID,
		"phone_number_id": c.PhoneNumberID,
		"display_phone":   c.DisplayPhone,
		columnStatus:      c.Status,
		"seat_id":         c.SeatID,
	}
}

// ConnectWhatsApp seals the number's secrets and writes its live row, its seat
// and its audit row. The verify token is made here and returned once, because
// the admin must paste it into Meta and nothing reads it back.
func (s *ChannelStore) ConnectWhatsApp(ctx context.Context, req ConnectWhatsAppRequest) (WhatsAppConnection, string, error) {
	verifyToken, err := newVerifyToken()
	if err != nil {
		return WhatsAppConnection{}, "", err
	}
	secrets, err := json.Marshal(whatsappSecrets{AppSecret: req.AppSecret, VerifyToken: verifyToken})
	if err != nil {
		return WhatsAppConnection{}, "", fmt.Errorf("capture: encoding the webhook secrets: %w", err)
	}
	var out WhatsAppConnection
	err = s.connectChatAccount(ctx, chatAccount{
		Provider: whatsapp.Provider, Account: req.PhoneNumberID, Channel: req.WABAID, Label: req.DisplayPhone,
		ReaderTeam: req.ReaderTeam, Token: req.AccessToken, Events: string(secrets), Noun: "phone number",
		Required: map[string]string{
			"the WhatsApp Business Account id": req.WABAID, "the phone number id": req.PhoneNumberID,
			"the display phone number": req.DisplayPhone, "the access token": req.AccessToken, "the app secret": req.AppSecret,
		},
	}, func(tx pgx.Tx, id ids.UUID) (map[string]any, error) {
		var err error
		out, err = readWhatsApp(ctx, tx, id)
		return whatsappAuditImage(out), err
	})
	if err != nil {
		return WhatsAppConnection{}, "", err
	}
	return out, verifyToken, nil
}

func readWhatsApp(ctx context.Context, tx pgx.Tx, id ids.UUID) (WhatsAppConnection, error) {
	return scanWhatsAppConnection(tx.QueryRow(ctx, `SELECT `+whatsappColumns+` FROM channel_connection WHERE id = $1`, id))
}

func newVerifyToken() (string, error) {
	raw := make([]byte, verifyTokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("capture: making a webhook verify token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// ListWhatsAppConnections returns the live WhatsApp numbers, newest first.
func (s *ChannelStore) ListWhatsAppConnections(ctx context.Context) ([]WhatsAppConnection, error) {
	return listChatAccounts[WhatsAppConnection](ctx, s, whatsapp.Provider, whatsappColumns)
}

// DisconnectWhatsApp archives the number, ends its seat and destroys its sealed
// secrets. From the archive on the webhook route stores nothing for it.
func (s *ChannelStore) DisconnectWhatsApp(ctx context.Context, id ids.UUID) error {
	return s.disconnectChatAccount(ctx, whatsapp.Provider, id, func(tx pgx.Tx, id ids.UUID) (map[string]any, error) {
		current, err := readWhatsApp(ctx, tx, id)
		return whatsappAuditImage(current), err
	})
}

// ChangeWhatsAppVisibility sets who may read the messages of a connected WhatsApp
// number, as ChangeBusinessVisibility does for a Telegram Business account.
func (s *ChannelStore) ChangeWhatsAppVisibility(ctx context.Context, id ids.UUID, posture string, confirmed bool) (WhatsAppConnection, error) {
	var out WhatsAppConnection
	err := s.changeChatAccountVisibility(ctx, whatsapp.Provider, id, posture, confirmed, func(tx pgx.Tx) error {
		var err error
		out, err = readWhatsApp(ctx, tx, id)
		return err
	})
	return out, err
}

// WhatsAppEndpoint is one live number with the secrets its webhook items are
// checked with. It is what the unauthenticated webhook route reads, so it exists
// only for that route and never reaches a response body.
type WhatsAppEndpoint struct {
	ConnectionID  ids.UUID
	PhoneNumberID string
	// WABAID is the WhatsApp Business Account the number belongs to.
	WABAID      string
	AppSecret   string
	VerifyToken string
}

// WhatsAppEdge reads the live numbers' webhook secrets. It carries no principal
// gate because Meta's call has none; the request's signature or verify token is
// what the caller checks against what this returns.
type WhatsAppEdge struct {
	db    *database.DB
	vault func() keyvault.Vault
	log   *slog.Logger
}

// NewWhatsAppEdge reads through vault, which may be late-bound.
func NewWhatsAppEdge(db *database.DB, vault func() keyvault.Vault, log *slog.Logger) *WhatsAppEdge {
	return &WhatsAppEdge{db: db, vault: vault, log: log}
}

// LiveEndpoints returns every number that still receives, with its unsealed secrets.
func (e *WhatsAppEdge) LiveEndpoints(ctx context.Context) ([]WhatsAppEndpoint, error) {
	live, err := liveChatEndpoints[whatsappSecrets](ctx, e.db, e.vault, e.log, whatsapp.Provider)
	if err != nil {
		return nil, err
	}
	out := make([]WhatsAppEndpoint, 0, len(live))
	for _, l := range live {
		out = append(out, WhatsAppEndpoint{
			ConnectionID: l.ConnectionID, PhoneNumberID: l.Account, WABAID: l.Channel,
			AppSecret: l.Secrets.AppSecret, VerifyToken: l.Secrets.VerifyToken,
		})
	}
	return out, nil
}
