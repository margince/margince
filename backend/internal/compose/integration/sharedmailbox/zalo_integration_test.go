// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package sharedmailbox

// A contact's mail and Zalo messages follow the contact's sharing the same way.
// The mail comes through a shared mailbox and the chat through a Zalo OA's team
// seat. The chat is filed through the real Zalo mapping and capture.FileChatMessage.

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/capture/zalo"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

const (
	zaloOAID      = "9100"
	zaloCustomer  = "5550100"
	zaloChatBody  = "is the order on its way?"
	zaloMailTitle = "order 42 for the warehouse"
)

// zaloSeat gives an OA its seat as the connecting admin does, in the posture asked.
func (s *sharedMailboxSuite) zaloSeat(t *testing.T, posture string) ids.UUID {
	t.Helper()
	ctx := s.admin()
	var seat ids.UUID
	if err := database.WithWorkspaceTx(ctx, s.e.Pool, func(tx pgx.Tx) error {
		var err error
		seat, err = capture.EnsureChatSeatTx(ctx, tx, capture.ChatSeat{
			Provider: zalo.Provider, Account: zaloOAID, ReaderTeam: s.e.Team1, By: s.e.AdminUser,
		})
		return err
	}); err != nil {
		t.Fatalf("seating the OA: %v", err)
	}
	s.e.WsExec(t, `UPDATE capture_connection SET mail_posture = $2 WHERE id = $1`, seat, posture)
	return seat
}

func (s *sharedMailboxSuite) fileZalo(t *testing.T, seat ids.UUID) {
	t.Helper()
	ctx := principal.WithWorkspaceID(context.Background(), s.e.WS)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalConnector, ID: zalo.CapturedBy,
		Permissions: principal.Permissions{
			RoleKeys: []string{"channel"},
			Objects: map[string]principal.ObjectGrant{
				"activity": {Create: true}, "contact": {Create: true}, "company": {Create: true, Read: true},
			},
			RowScope: principal.RowScopeAll,
		},
	})
	ev := zalo.Event{
		Kind: zalo.KindMessage, Name: "user_send_text", OAID: zaloOAID, UserID: zaloCustomer, OccurredAt: clocktest.Now(t).Add(-time.Minute),
		Message: &zalo.Message{ID: "msg-acc-1", Direction: zalo.Inbound, Type: zalo.MessageText, Text: zaloChatBody},
	}
	chat, err := zalo.ChatMessage(ev)
	if err != nil {
		t.Fatal(err)
	}
	link := connector.ChatLink{AccountID: zaloOAID, Ownership: connector.ChatOwnedByWorkspace, SeatID: seat}
	if _, err := capture.FileChatMessage(ctx, compose.NewCaptureSink(s.e.Pool, compose.CaptureConfig{}), zalo.ChatBinding(ev, link), chat); err != nil {
		t.Fatalf("filing the Zalo message: %v", err)
	}
}

// timelineHas reports whether the reader's view of the contact's timeline holds
// text. A contact the reader may not open answers 404 for its whole timeline.
func (s *sharedMailboxSuite) timelineHas(t *testing.T, reader ids.UUID, teams []ids.UUID, text string) bool {
	t.Helper()
	perms := principal.Permissions{
		Objects:  map[string]principal.ObjectGrant{"activity": {Read: true}, "contact": {Read: true}, "company": {Read: true}},
		RowScope: principal.RowScopeAll,
	}
	rec := s.call(s.e.As(reader, teams, perms), t, http.MethodGet, fmt.Sprintf("/activities?entity_type=contact&entity_id=%s", s.anna), nil)
	switch rec.Code {
	case http.StatusNotFound:
		return false
	case http.StatusOK:
		return strings.Contains(rec.Body.String(), text)
	}
	t.Fatalf("the timeline answered %d: %s", rec.Code, rec.Body)
	return false
}

func (s *sharedMailboxSuite) bindZaloCustomer(t *testing.T) {
	t.Helper()
	s.e.WsExec(t, `INSERT INTO contact_channel_identity (contact_id, provider, account_scope, channel_user_id, source, captured_by)
		VALUES ($1, 'zalo_oa', $2, $3, 'zalo_oa', 'connector:zalo_oa')`, s.anna, zaloOAID, zaloCustomer)
}

func TestMailAndZaloOnOneContactFollowTheContactsSharingTheSameWay(t *testing.T) {
	s := newSharedMailboxSuite(t)
	e := s.e
	box := s.connectOpen(t)
	seat := s.zaloSeat(t, "shared")
	s.bindZaloCustomer(t)
	s.syncOne(t, box, orderMail("acceptance@acme.example", salesMailbox), zaloMailTitle)
	s.fileZalo(t, seat)

	team1, team2 := []ids.UUID{e.Team1}, []ids.UUID{e.Team2}
	same := func(label string, reader ids.UUID, teams []ids.UUID, want bool) {
		t.Helper()
		mail, chat := s.timelineHas(t, reader, teams, zaloMailTitle), s.timelineHas(t, reader, teams, zaloChatBody)
		if mail != want || chat != want {
			t.Errorf("%s: sees the mail %v and the Zalo message %v, want both %v", label, mail, chat, want)
		}
	}

	same("a colleague, contact open to all", e.Rep3, team2, true)
	e.WsExec(t, `UPDATE contact SET visibility = 'owner', owner_id = $2 WHERE id = $1`, s.anna, e.Rep2)
	same("the contact's owner, contact private", e.Rep2, team1, true)
	same("a colleague, contact private", e.Rep3, team2, false)
	e.WsExec(t, `UPDATE contact SET visibility = 'workspace' WHERE id = $1`, s.anna)
	same("a colleague, contact open again", e.Rep3, team2, true)
}

func TestAHeldZaloSeatKeepsItsMessagesFromColleaguesOutsideTheTeamOnASharedContact(t *testing.T) {
	s := newSharedMailboxSuite(t)
	e := s.e
	seat := s.zaloSeat(t, "held")
	s.bindZaloCustomer(t)
	s.fileZalo(t, seat)

	if !s.timelineHas(t, e.Rep1, []ids.UUID{e.Team1}, zaloChatBody) {
		t.Error("a member of the reader team cannot read the held message on the shared contact")
	}
	if s.timelineHas(t, e.Rep3, []ids.UUID{e.Team2}, zaloChatBody) {
		t.Error("a colleague outside the reader team reads a held message on a contact open to the workspace")
	}
}
