// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package sharedmailbox

// A contact's mail and WhatsApp messages follow the contact's sharing the same
// way. The mail comes through a shared mailbox and the chat through a WhatsApp
// number's team seat, both open to the workspace. The chat is filed through the
// real WhatsApp mapping and capture.FileChatMessage.

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/capture/whatsapp"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

const (
	whatsAppPhoneID   = "7100"
	whatsAppCustomer  = "491701234567"
	whatsAppChatBody  = "is the order on its way?"
	whatsAppMailTitle = "order 42 for the warehouse"
)

// whatsAppSeat gives a number its seat as the approving admin does, open to the workspace.
func (s *sharedMailboxSuite) whatsAppSeat(t *testing.T) ids.UUID {
	t.Helper()
	ctx := s.admin()
	var seat ids.UUID
	if err := database.WithTx(ctx, s.e.Pool, func(tx pgx.Tx) error {
		var err error
		seat, err = capture.EnsureChatSeatTx(ctx, tx, capture.ChatSeat{
			Provider: whatsapp.Provider, Account: whatsAppPhoneID, ReaderTeam: s.e.Team1, By: s.e.AdminUser,
		})
		return err
	}); err != nil {
		t.Fatalf("seating the number: %v", err)
	}
	s.e.WsExec(t, `UPDATE capture_connection SET mail_posture = 'shared' WHERE id = $1`, seat)
	return seat
}

func (s *sharedMailboxSuite) fileWhatsApp(t *testing.T, seat ids.UUID) {
	t.Helper()
	ctx := context.Background()
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalConnector, ID: whatsapp.CapturedBy,
		Permissions: principal.Permissions{
			RoleKeys: []string{"channel"},
			Objects: map[string]principal.ObjectGrant{
				"activity": {Create: true}, "contact": {Create: true}, "company": {Create: true, Read: true},
			},
			RowScope: principal.RowScopeAll,
		},
	})
	msg := whatsapp.Message{
		WABAID: "WABA1", PhoneNumberID: whatsAppPhoneID, ID: "wamid.ACC1", Direction: whatsapp.Inbound,
		CounterpartyPhone: whatsAppCustomer, DisplayName: "Anna", At: clocktest.Now(t), Kind: whatsapp.KindText, Text: whatsAppChatBody,
	}
	chat, err := whatsapp.ChatMessage(msg)
	if err != nil {
		t.Fatal(err)
	}
	link := connector.ChatLink{AccountID: whatsAppPhoneID, Ownership: connector.ChatOwnedByWorkspace, SeatID: seat}
	if _, err := capture.FileChatMessage(ctx, compose.NewCaptureSink(s.e.Pool, compose.CaptureConfig{}), whatsapp.ChatBinding(msg, link), chat); err != nil {
		t.Fatalf("filing the WhatsApp message: %v", err)
	}
}

func TestMailAndWhatsAppOnOneContactFollowTheContactsSharingTheSameWay(t *testing.T) {
	s := newSharedMailboxSuite(t)
	e := s.e
	box := s.connectOpen(t)
	seat := s.whatsAppSeat(t)
	e.WsExec(t, `INSERT INTO contact_channel_identity (contact_id, provider, channel_user_id, source, captured_by)
		VALUES ($1, 'whatsapp', $2, 'whatsapp', 'connector:whatsapp')`, s.anna, whatsAppCustomer)
	s.syncOne(t, box, orderMail("acceptance@acme.example", salesMailbox), whatsAppMailTitle)
	s.fileWhatsApp(t, seat)

	team1, team2 := []ids.UUID{e.Team1}, []ids.UUID{e.Team2}
	// A contact the reader may not open answers 404 for its whole timeline.
	timelineHas := func(reader ids.UUID, teams []ids.UUID, text string) bool {
		perms := principal.Permissions{
			Objects:  map[string]principal.ObjectGrant{"activity": {Read: true}, "contact": {Read: true}, "company": {Read: true}},
			RowScope: principal.RowScopeAll,
		}
		rec := s.call(e.As(reader, teams, perms), t, http.MethodGet, fmt.Sprintf("/activities?entity_type=contact&entity_id=%s", s.anna), nil)
		switch rec.Code {
		case http.StatusNotFound:
			return false
		case http.StatusOK:
			return strings.Contains(rec.Body.String(), text)
		}
		t.Fatalf("the timeline answered %d: %s", rec.Code, rec.Body)
		return false
	}
	sees := func(reader ids.UUID, teams []ids.UUID) (mail, chat bool) {
		return timelineHas(reader, teams, whatsAppMailTitle), timelineHas(reader, teams, whatsAppChatBody)
	}
	same := func(label string, reader ids.UUID, teams []ids.UUID, want bool) {
		t.Helper()
		mail, chat := sees(reader, teams)
		if mail != want || chat != want {
			t.Errorf("%s: sees the mail %v and the WhatsApp message %v, want both %v", label, mail, chat, want)
		}
	}

	same("a colleague, contact open to all", e.Rep3, team2, true)
	e.WsExec(t, `UPDATE contact SET visibility = 'owner', owner_id = $2 WHERE id = $1`, s.anna, e.Rep2)
	same("the contact's owner, contact private", e.Rep2, team1, true)
	same("a colleague, contact private", e.Rep3, team2, false)
	e.WsExec(t, `UPDATE contact SET visibility = 'workspace' WHERE id = $1`, s.anna)
	same("a colleague, contact open again", e.Rep3, team2, true)
}
