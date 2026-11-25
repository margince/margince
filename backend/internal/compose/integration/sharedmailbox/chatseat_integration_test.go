// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package sharedmailbox

// A business chat account files through a seat, and the seat's posture decides
// who reads its messages as a mailbox's posture does for mail. These cases file
// real chat messages through capture.FileChatMessage and the production sink,
// and read them back through the same timeline a colleague opens.

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

const chatSeatProvider = "telegram_business"

// seatFor gives a business account its seat as the approving admin does.
func (s *sharedMailboxSuite) seatFor(t *testing.T, account string, team ids.UUID, owner ids.UUID) ids.UUID {
	t.Helper()
	ctx := s.admin()
	var seat ids.UUID
	err := database.WithWorkspaceTx(ctx, s.e.Pool, func(tx pgx.Tx) error {
		var err error
		seat, err = capture.EnsureChatSeatTx(ctx, tx, capture.ChatSeat{
			Provider: chatSeatProvider, Account: account, ReaderTeam: team, Owner: owner, By: s.e.AdminUser,
		})
		return err
	})
	if err != nil {
		t.Fatalf("seating account %s: %v", account, err)
	}
	return seat
}

// bindAnnaToChat makes the chat user 990100 the contact the suite's mail files under.
func (s *sharedMailboxSuite) bindAnnaToChat(t *testing.T) {
	t.Helper()
	s.e.WsExec(t, `INSERT INTO contact_channel_identity (contact_id, provider, channel_user_id, username, source, captured_by)
		VALUES ($1, 'telegram', '990100', 'anna', 'telegram', 'connector:telegram')`, s.anna)
}

// fileChat files one inbound chat message the way the Telegram ingest worker
// does, on the account binding given, and answers its activity id.
func (s *sharedMailboxSuite) fileChat(t *testing.T, binding connector.ChatBinding, messageID, body string) ids.UUID {
	t.Helper()
	ctx := principal.WithWorkspaceID(context.Background(), s.e.WS)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalConnector, ID: "connector:telegram",
		Permissions: principal.Permissions{
			RoleKeys: []string{"channel"},
			Objects: map[string]principal.ObjectGrant{
				"activity": {Create: true}, "contact": {Create: true}, "company": {Create: true, Read: true},
			},
			RowScope: principal.RowScopeAll,
		},
	})
	binding.Provider, binding.CapturedBy = "telegram", "connector:telegram"
	if binding.ThreadScope == "" {
		binding.KeyScope, binding.ThreadScope = "42:biz:880001", "biz:880001"
	}
	ref, err := capture.FileChatMessage(ctx, compose.NewCaptureSink(s.e.Pool, compose.CaptureConfig{}), binding, connector.ChatMessage{
		ConversationID: "990100", IdempotencyKey: messageID, Direction: connector.DirectionInbound,
		Counterparty: connector.ChatParty{UserID: "990100", DisplayName: "Anna"}, Body: body, OccurredAt: clocktest.Now(t),
	})
	if err != nil {
		t.Fatalf("filing chat message %s: %v", messageID, err)
	}
	return ref.ID
}

// readsChat answers whether reader finds a chat message with this body on the
// contact's timeline.
func (s *sharedMailboxSuite) readsChat(t *testing.T, reader ids.UUID, teams []ids.UUID, body string) bool {
	t.Helper()
	perms := principal.Permissions{
		Objects:  map[string]principal.ObjectGrant{"activity": {Read: true}, "contact": {Read: true}, "company": {Read: true}},
		RowScope: principal.RowScopeAll,
	}
	rec := s.call(s.e.As(reader, teams, perms), t, http.MethodGet, fmt.Sprintf("/activities?entity_type=contact&entity_id=%s", s.anna), nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("the timeline answered %d: %s", rec.Code, rec.Body)
	}
	return strings.Contains(rec.Body.String(), body)
}

func (s *sharedMailboxSuite) audienceOf(t *testing.T, activity ids.UUID) string {
	t.Helper()
	return s.e.WsScalar(t, `SELECT audience FROM activity WHERE id = $1`, activity)
}

func TestEverySharedMailboxRouteAnswersNotFoundForAChatSeat(t *testing.T) {
	s := newSharedMailboxSuite(t)
	seat := s.seatFor(t, "880002", s.e.Team1, ids.Nil)
	for _, route := range []struct {
		method, path string
		body         map[string]any
	}{
		{http.MethodPatch, "/shared-mailboxes/" + seat.String(), map[string]any{"mail_visibility": "held", "confirm": true}},
		{http.MethodPost, "/shared-mailboxes/" + seat.String() + "/disconnect", nil},
	} {
		if rec := s.call(s.admin(), t, route.method, route.path, route.body); rec.Code != http.StatusNotFound {
			t.Errorf("%s %s on a chat seat answered %d, want 404: %s", route.method, route.path, rec.Code, rec.Body)
		}
	}
	if got := s.e.WsScalar(t, `SELECT status FROM capture_connection WHERE id = $1`, seat); got != "connected" {
		t.Errorf("the refused routes left the seat %q, want connected", got)
	}
}

func TestAHeldCompanyAccountKeepsItsMessagesFromColleaguesUntilTheAdminSharesIt(t *testing.T) {
	s := newSharedMailboxSuite(t)
	e := s.e
	seat := s.seatFor(t, "880001", e.Team1, ids.Nil)
	s.bindAnnaToChat(t)
	team := []ids.UUID{e.Team1}
	outsider := s.outsiderSeat(t)
	company := connector.ChatBinding{Ownership: connector.ChatOwnedByWorkspace, SeatID: seat}

	if got := e.WsScalar(t, `SELECT mail_posture || '/' || coalesce(user_id::text, 'team') FROM capture_connection WHERE id = $1`, seat); got != "held/team" {
		t.Fatalf("a new company seat is %q, want held and read by a team", got)
	}
	first := s.fileChat(t, company, "1", "the held chat message")
	if got := s.audienceOf(t, first); got != "participants" {
		t.Fatalf("a message on a held company seat is %q, want participants", got)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM capture_import WHERE activity_id = $1 AND connection_id = $2 AND user_id IS NULL AND posture_at_import = 'held'`, first, seat); got != 1 {
		t.Fatalf("the message carries %d import rows from the seat under held, want 1", got)
	}
	if !s.readsChat(t, e.Rep2, team, "the held chat message") {
		t.Error("a member of the reader team cannot read a held message")
	}
	if s.readsChat(t, e.Rep3, []ids.UUID{e.Team2}, "the held chat message") || s.readsChat(t, outsider, nil, "the held chat message") {
		t.Error("a colleague outside the reader team reads a held message on a contact everyone can open")
	}

	// The posture change itself is the business connection's door, tested in the
	// channels lane; this case needs only a seat that is open.
	e.WsExec(t, `UPDATE capture_connection SET mail_posture = 'shared' WHERE id = $1`, seat)

	second := s.fileChat(t, company, "2", "the shared chat message")
	if got := s.audienceOf(t, second); got != "workspace" {
		t.Fatalf("a message on a shared company seat is %q, want workspace", got)
	}
	if !s.readsChat(t, e.Rep3, []ids.UUID{e.Team2}, "the shared chat message") || !s.readsChat(t, outsider, nil, "the shared chat message") {
		t.Error("a colleague cannot read a message on a shared seat although the contact is open to them")
	}
	if s.readsChat(t, e.Rep3, []ids.UUID{e.Team2}, "the held chat message") {
		t.Error("opening the seat published a message that arrived while it was held")
	}
	if got := e.WsCount(t, `SELECT count(*) FROM capture_connection WHERE provider = $1`, chatSeatProvider); got != 1 {
		t.Fatalf("the account has %d seats, want one", got)
	}
}

func TestAMemberAccountsMessageIsBornHeldUnderClassified(t *testing.T) {
	s := newSharedMailboxSuite(t)
	e := s.e
	seat := s.seatFor(t, "880002", ids.Nil, e.Rep3)
	s.bindAnnaToChat(t)

	if got := e.WsScalar(t, `SELECT mail_posture FROM capture_connection WHERE id = $1 AND user_id = $2`, seat, e.Rep3); got != "classified" {
		t.Fatalf("a member's own account starts %q, want classified", got)
	}
	member := connector.ChatBinding{Ownership: connector.ChatOwnedByMember, OwnerUserID: e.Rep3, SeatID: seat}
	chat := s.fileChat(t, member, "1", "the member's own chat")
	if got := s.audienceOf(t, chat); got != "participants" {
		t.Fatalf("a message on a classified member seat is %q, want participants", got)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM capture_import WHERE activity_id = $1 AND user_id = $2 AND connection_id IS NULL AND posture_at_import = 'classified'
		AND delivered_by <> '{}'`, chat, e.Rep3); got != 1 {
		t.Fatalf("the member's import row is missing, or it does not name the seat that delivered it (%d rows)", got)
	}
	if !s.readsChat(t, e.Rep3, []ids.UUID{e.Team2}, "the member's own chat") {
		t.Error("the member cannot read their own chat")
	}
	if s.readsChat(t, e.Rep2, []ids.UUID{e.Team1}, "the member's own chat") {
		t.Error("a colleague reads a member's classified chat")
	}

	noSeat := connector.ChatBinding{Ownership: connector.ChatOwnedByMember, OwnerUserID: e.Rep3, KeyScope: "42", ThreadScope: "42"}
	orphan := s.fileChat(t, noSeat, "2", "a chat with no seat")
	if got := s.audienceOf(t, orphan); got != "participants" {
		t.Fatalf("a member's message with no seat is %q, want participants", got)
	}
}

func TestEndingASeatKeepsItsMessagesWithTheReaderTeamAndAReconnectWakesIt(t *testing.T) {
	s := newSharedMailboxSuite(t)
	e := s.e
	seat := s.seatFor(t, "880003", e.Team1, ids.Nil)
	s.bindAnnaToChat(t)
	chat := s.fileChat(t, connector.ChatBinding{Ownership: connector.ChatOwnedByWorkspace, SeatID: seat}, "1", "before the disconnect")

	err := database.WithWorkspaceTx(s.admin(), e.Pool, func(tx pgx.Tx) error { return capture.EndChatSeatTx(s.admin(), tx, seat) })
	if err != nil {
		t.Fatalf("ending the seat: %v", err)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM capture_connection WHERE id = $1 AND status = 'disconnected' AND reader_team_id = $2`, seat, e.Team1); got != 1 {
		t.Fatal("an ended seat must keep its row and its readers")
	}
	if !s.readsChat(t, e.Rep2, []ids.UUID{e.Team1}, "before the disconnect") {
		t.Error("the reader team lost a message when the seat ended")
	}
	if again := s.seatFor(t, "880003", ids.Nil, ids.Nil); again != seat {
		t.Fatalf("the same account got seat %s after a reconnect, want %s", again, seat)
	}
	if got := e.WsScalar(t, `SELECT status FROM capture_connection WHERE id = $1`, seat); got != "connected" {
		t.Fatalf("a reconnected seat is %q, want connected", got)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM capture_import WHERE activity_id = $1 AND connection_id = $2`, chat, seat); got != 1 {
		t.Fatal("the message lost its import row")
	}
}
