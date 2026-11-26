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
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/capture/whatsapp"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
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

// ConnectWhatsApp seals the number's secrets and writes its live row with its
// audit row. A number connects once: the unique index on (provider, account_id)
// refuses a second live row. The verify token is made here and returned once, because
// the admin must paste it into Meta and nothing reads it back.
func (s *ChannelStore) ConnectWhatsApp(ctx context.Context, req ConnectWhatsAppRequest) (WhatsAppConnection, string, error) {
	if err := auth.Require(ctx, channelConnectionObject, principal.ActionCreate); err != nil {
		return WhatsAppConnection{}, "", err
	}
	if err := auth.RequireHuman(ctx); err != nil {
		return WhatsAppConnection{}, "", err
	}
	if err := validateWhatsAppRequest(req); err != nil {
		return WhatsAppConnection{}, "", err
	}
	if s.vault == nil {
		return WhatsAppConnection{}, "", fmt.Errorf("configure a credential store for this installation, so the secrets can be sealed: %w",
			ErrChannelWiringIncomplete)
	}
	ws, ok := principal.WorkspaceID(ctx)
	if !ok {
		return WhatsAppConnection{}, "", errors.New("capture: whatsapp connect called outside a workspace context")
	}
	connectedBy, err := channelActor(ctx)
	if err != nil {
		return WhatsAppConnection{}, "", err
	}
	verifyToken, err := newVerifyToken()
	if err != nil {
		return WhatsAppConnection{}, "", err
	}
	refs, err := s.sealWhatsAppSecrets(ctx, ws, req, verifyToken)
	if err != nil {
		return WhatsAppConnection{}, "", err
	}
	conn, err := s.insertWhatsApp(ctx, req, refs, connectedBy)
	if err != nil {
		// A lost race for the unique index proves no row persisted, so the refs
		// are safe to destroy. Any other error leaves the commit outcome open.
		if _, unique := storekit.UniqueViolation(err); unique {
			s.destroyDetached(ctx, ws, "whatsapp-connect-lost-race", refs.token, refs.events)
			return WhatsAppConnection{}, "", fmt.Errorf("this phone number is already connected; disconnect it first: %w", apperrors.ErrConflict)
		}
		var teamErr *ReaderTeamError
		var reconnectErr *SharedReconnectError
		if errors.As(err, &teamErr) || errors.As(err, &reconnectErr) {
			// The transaction rolled back, so no row names the refs.
			s.destroyDetached(ctx, ws, "whatsapp-connect-refused", refs.token, refs.events)
			return WhatsAppConnection{}, "", err
		}
		return WhatsAppConnection{}, "", fmt.Errorf("capture: connecting a whatsapp number: %w", err)
	}
	return conn, verifyToken, nil
}

func validateWhatsAppRequest(req ConnectWhatsAppRequest) error {
	if req.ReaderTeam.IsZero() {
		return &ReaderTeamError{missing: true}
	}
	for what, value := range map[string]string{
		"the WhatsApp Business Account id": req.WABAID, "the phone number id": req.PhoneNumberID,
		"the display phone number": req.DisplayPhone, "the access token": req.AccessToken, "the app secret": req.AppSecret,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("enter %s: %w", what, apperrors.ErrInvalidArgument)
		}
	}
	return nil
}

func newVerifyToken() (string, error) {
	raw := make([]byte, verifyTokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("capture: making a webhook verify token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func (s *ChannelStore) sealWhatsAppSecrets(ctx context.Context, ws ids.UUID, req ConnectWhatsAppRequest, verifyToken string) (connectionRefs, error) {
	pair, err := json.Marshal(whatsappSecrets{AppSecret: req.AppSecret, VerifyToken: verifyToken})
	if err != nil {
		return connectionRefs{}, fmt.Errorf("capture: encoding the webhook secrets: %w", err)
	}
	return s.sealConnectionSecrets(ctx, ws, req.AccessToken, string(pair))
}

func (s *ChannelStore) insertWhatsApp(ctx context.Context, req ConnectWhatsAppRequest, refs connectionRefs, connectedBy ids.UUID) (WhatsAppConnection, error) {
	var out WhatsAppConnection
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		seat, err := EnsureChatSeatTx(ctx, tx, ChatSeat{
			Provider: whatsapp.Provider, Account: req.PhoneNumberID, ReaderTeam: req.ReaderTeam, By: connectedBy,
		})
		if err != nil {
			return err
		}
		out, err = scanWhatsAppConnection(tx.QueryRow(ctx, `
			WITH made AS (
			  INSERT INTO channel_connection
			    (provider, channel_id, channel_label, account_id, credential_ref, events_credential_ref,
			     status, connected_by, approved_at, seat_id)
			  VALUES ($1, $2, $3, $4, $5, $6, $7, $8, now(), $9)
			  RETURNING *)
			SELECT `+whatsappColumns+` FROM made channel_connection`,
			whatsapp.Provider, req.WABAID, req.DisplayPhone, req.PhoneNumberID,
			string(refs.token), string(refs.events), channelStatusConnected, connectedBy, seat))
		if err != nil {
			return err
		}
		return auditLifecycle(ctx, tx, "create", channelConnectionObject, out.ID, nil, whatsappAuditImage(out))
	})
	return out, err
}

// ListWhatsAppConnections returns the live WhatsApp numbers, newest first.
func (s *ChannelStore) ListWhatsAppConnections(ctx context.Context) ([]WhatsAppConnection, error) {
	if err := requireBusinessAdmin(ctx, principal.ActionUpdate); err != nil {
		return nil, err
	}
	var out []WhatsAppConnection
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT `+whatsappColumns+` FROM channel_connection
			 WHERE archived_at IS NULL AND `+whatsappRow+` ORDER BY created_at DESC, id DESC`)
		if err != nil {
			return err
		}
		out, err = pgx.CollectRows(rows, pgx.RowToStructByPos[WhatsAppConnection])
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("capture: listing whatsapp connections: %w", err)
	}
	return out, nil
}

// DisconnectWhatsApp archives the number and destroys its sealed secrets. From
// the archive on the webhook route stores nothing for it. What it filed stays.
func (s *ChannelStore) DisconnectWhatsApp(ctx context.Context, id ids.UUID) error {
	if err := requireBusinessAdmin(ctx, principal.ActionDelete); err != nil {
		return err
	}
	if s.vault == nil {
		return fmt.Errorf("configure a credential store for this installation, so the sealed secrets can be destroyed: %w",
			ErrChannelWiringIncomplete)
	}
	ws, ok := principal.WorkspaceID(ctx)
	if !ok {
		return errors.New("capture: whatsapp disconnect called outside a workspace context")
	}
	var refs connectionRefs
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		var current WhatsAppConnection
		var tokenRef, eventsRef string
		err := tx.QueryRow(ctx, `
			SELECT `+whatsappColumns+`, credential_ref, coalesce(events_credential_ref, '') FROM channel_connection
			 WHERE id = $1 AND archived_at IS NULL AND `+whatsappRow+` FOR UPDATE`, id).
			Scan(&current.ID, &current.WABAID, &current.PhoneNumberID, &current.DisplayPhone, &current.Status,
				&current.ConnectedAt, &current.SeatID, &current.ReaderTeam, &current.Visibility, &tokenRef, &eventsRef)
		if errors.Is(err, pgx.ErrNoRows) {
			return apperrors.ErrNotFound
		}
		if err != nil {
			return err
		}
		after := current
		after.Status = channelStatusDisconnected
		if _, err := tx.Exec(ctx, `UPDATE channel_connection SET status = $2, archived_at = now() WHERE id = $1`,
			id, after.Status); err != nil {
			return err
		}
		refs = connectionRefs{token: keyvault.Ref(tokenRef), events: keyvault.Ref(eventsRef)}
		if current.SeatID != nil {
			if err := EndChatSeatTx(ctx, tx, *current.SeatID); err != nil {
				return err
			}
		}
		return auditLifecycle(ctx, tx, "archive", channelConnectionObject, id, whatsappAuditImage(current), whatsappAuditImage(after))
	})
	if errors.Is(err, apperrors.ErrNotFound) {
		return err
	}
	if err != nil {
		return fmt.Errorf("capture: disconnecting a whatsapp number: %w", err)
	}
	s.destroyDetached(ctx, ws, "whatsapp-disconnected", refs.token, refs.events)
	return nil
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

// LiveEndpoints returns every connected number with its unsealed secrets. A number
// whose secrets cannot be read is left out and logged by id, so one broken seal
// does not stop the others' deliveries.
func (e *WhatsAppEdge) LiveEndpoints(ctx context.Context) ([]WhatsAppEndpoint, error) {
	vault := e.vault()
	ws, ok := principal.WorkspaceID(ctx)
	if vault == nil || !ok {
		return nil, ErrChannelWiringIncomplete
	}
	type endpointRow struct {
		id      ids.UUID
		phone   string
		waba    string
		sealRef string
	}
	var rows []endpointRow
	err := e.db.Tx(ctx, func(tx pgx.Tx) error {
		res, err := tx.Query(ctx, `SELECT id, account_id, channel_id, events_credential_ref FROM channel_connection
			 WHERE archived_at IS NULL AND status = $1 AND events_credential_ref IS NOT NULL AND `+whatsappRow+`
			 ORDER BY created_at, id`, channelStatusConnected)
		if err != nil {
			return err
		}
		defer res.Close()
		for res.Next() {
			var r endpointRow
			if err := res.Scan(&r.id, &r.phone, &r.waba, &r.sealRef); err != nil {
				return err
			}
			rows = append(rows, r)
		}
		return res.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("capture: reading the live whatsapp numbers: %w", err)
	}
	out := make([]WhatsAppEndpoint, 0, len(rows))
	for _, r := range rows {
		sealed, err := vault.Get(ctx, ids.From[ids.WorkspaceKind](ws), keyvault.Ref(r.sealRef))
		var secrets whatsappSecrets
		if err == nil {
			err = json.Unmarshal(sealed, &secrets)
		}
		if err != nil {
			e.log.WarnContext(ctx, "whatsapp: the sealed webhook secrets of a connection cannot be read", "connection", r.id.String(), "err", err)
			continue
		}
		out = append(out, WhatsAppEndpoint{ConnectionID: r.id, PhoneNumberID: r.phone, WABAID: r.waba, AppSecret: secrets.AppSecret, VerifyToken: secrets.VerifyToken})
	}
	return out, nil
}
