// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package channels

// Telegram Business connections. An account owner links the workspace's bot
// inside Telegram, and nothing from that account may be stored until an admin
// approves it. These cases drive the real poll worker over the faked Bot API and
// the real store for the admin's side.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// businessUpdate is one business_connection update in the shape the Bot API posts.
type businessUpdate struct {
	updateID     int64
	connectionID string
	ownerID      int64
	firstName    string
	enabled      bool
}

func (u businessUpdate) body(t *testing.T) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"update_id": u.updateID,
		"business_connection": map[string]any{
			"id":           u.connectionID,
			"user":         map[string]any{"id": u.ownerID, "first_name": u.firstName, "username": "owner" + u.firstName},
			"user_chat_id": u.ownerID,
			"date":         telegramProviderDate,
			"is_enabled":   u.enabled,
		},
	})
	if err != nil {
		t.Fatalf("rendering the business connection update: %v", err)
	}
	return raw
}

// deliver hands one update to Telegram and runs the poll that collects it.
func (c *telegramEnv) deliver(t *testing.T, sub <-chan *river.Event, u businessUpdate) {
	t.Helper()
	c.api.hold(u.body(t))
	c.pollNow(t, sub, u.updateID+1)
}

type businessRow struct {
	id, status, channelID, label, parent, credentialRef string
	approved                                            bool
	version                                             int64
}

// businessRows reads the live account connections of the workspace, by account id.
func (c *telegramEnv) businessRow(t *testing.T, accountID string) (businessRow, bool) {
	t.Helper()
	var r businessRow
	var parent *string
	rows, err := c.Owner.Query(context.Background(), `
		SELECT id::text, status, channel_id, channel_label, parent_connection_id::text, credential_ref, approved_at IS NOT NULL, version
		  FROM channel_connection WHERE provider = 'telegram_business' AND account_id = $1 AND archived_at IS NULL`, accountID)
	if err != nil {
		t.Fatalf("reading the business connection: %v", err)
	}
	defer rows.Close()
	if !rows.Next() {
		return businessRow{}, false
	}
	if err := rows.Scan(&r.id, &r.status, &r.channelID, &r.label, &parent, &r.credentialRef, &r.approved, &r.version); err != nil {
		t.Fatalf("scanning the business connection: %v", err)
	}
	if parent != nil {
		r.parent = *parent
	}
	return r, true
}

func (c *telegramEnv) connectionAudits(t *testing.T, id string) int {
	t.Helper()
	return c.count(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'channel_connection' AND entity_id = $1`, id)
}

func TestABusinessConnectionWaitsForApprovalAndStoresNothing(t *testing.T) {
	c := setupTelegramConnected(t)
	runner, sub := newTelegramWorker(t, c, compose.JobRunnerConfig{})
	startTelegramWorker(t, runner)
	u := businessUpdate{updateID: 6001, connectionID: "bc-mia-1", ownerID: 880001, firstName: "Mia", enabled: true}

	c.deliver(t, sub, u)

	row, ok := c.businessRow(t, "880001")
	if !ok {
		t.Fatal("the business connection left no row")
	}
	if row.status != "pending_approval" || row.approved {
		t.Errorf("a new link is %q (approved=%v), want pending_approval and unapproved", row.status, row.approved)
	}
	if row.parent != c.conn.ID.String() || row.channelID != "bc-mia-1" || row.label != "Mia" || row.credentialRef != "" {
		t.Errorf("row = %+v, want it bound to bot %s, carrying Telegram's connection id and the owner's name and no credential", row, c.conn.ID)
	}
	if n := c.rawCaptures(t, u.updateID); n != 0 {
		t.Errorf("%d raw captures stored for a business connection, want none", n)
	}
	if n := c.ingestJobs(t); n != 0 {
		t.Errorf("%d ingest jobs for a business connection, want none", n)
	}
	if n := c.connectionAudits(t, row.id); n != 1 {
		t.Errorf("%d audit rows for the new link, want 1", n)
	}

	// Telegram sends the same update again when the cursor was lost.
	c.rewindPollCursor(t, 0)
	c.deliver(t, sub, u)
	again, _ := c.businessRow(t, "880001")
	if again.version != row.version || c.connectionAudits(t, row.id) != 1 {
		t.Errorf("a repeated update changed the link (version %d -> %d, %d audit rows)", row.version, again.version, c.connectionAudits(t, row.id))
	}
	if n := c.count(t, `SELECT count(*) FROM channel_connection WHERE provider = 'telegram_business' AND archived_at IS NULL`); n != 1 {
		t.Errorf("%d live account connections after a repeat, want 1", n)
	}
	if bot, ok := c.liveBot(t); !ok || bot != c.conn.ID.String() {
		t.Errorf("the live bot connection is %q, want it unchanged as %s", bot, c.conn.ID)
	}
}

// liveBot is the one live bot connection of the provider, which the business
// links beside it must not change or duplicate.
func (c *telegramEnv) liveBot(t *testing.T) (string, bool) {
	t.Helper()
	var id string
	err := c.Owner.QueryRow(context.Background(), `
		SELECT id::text FROM channel_connection
		 WHERE provider = 'telegram' AND archived_at IS NULL AND account_id IS NULL AND parent_connection_id IS NULL`).Scan(&id)
	return id, err == nil
}

func TestTwoBusinessAccountsOnOneBotGetTwoRows(t *testing.T) {
	c := setupTelegramConnected(t)
	runner, sub := newTelegramWorker(t, c, compose.JobRunnerConfig{})
	startTelegramWorker(t, runner)

	c.deliver(t, sub, businessUpdate{updateID: 6101, connectionID: "bc-a", ownerID: 880101, firstName: "Ann", enabled: true})
	c.deliver(t, sub, businessUpdate{updateID: 6102, connectionID: "bc-b", ownerID: 880102, firstName: "Ben", enabled: true})

	first, ok1 := c.businessRow(t, "880101")
	second, ok2 := c.businessRow(t, "880102")
	if !ok1 || !ok2 || first.id == second.id {
		t.Fatalf("rows = %+v / %+v, want one row per owner", first, second)
	}
	if first.channelID != "bc-a" || second.channelID != "bc-b" {
		t.Errorf("a row took the other owner's connection id: %q and %q", first.channelID, second.channelID)
	}
}

func TestTurningALinkOffAndOnReturnsItToItsApprovalState(t *testing.T) {
	c := setupTelegramConnected(t)
	runner, sub := newTelegramWorker(t, c, compose.JobRunnerConfig{})
	startTelegramWorker(t, runner)
	update := func(id int64, connection string, enabled bool) businessUpdate {
		return businessUpdate{updateID: id, connectionID: connection, ownerID: 880201, firstName: "Cy", enabled: enabled}
	}

	c.deliver(t, sub, update(6201, "bc-1", false))
	if _, ok := c.businessRow(t, "880201"); ok {
		t.Fatal("a link reported disabled before it was ever seen left a row")
	}
	c.deliver(t, sub, update(6202, "bc-1", true))
	c.deliver(t, sub, update(6203, "bc-1", false))
	if row, _ := c.businessRow(t, "880201"); row.status != "disconnected" {
		t.Fatalf("a disabled link is %q, want disconnected", row.status)
	}
	c.deliver(t, sub, update(6204, "bc-2", true))
	row, _ := c.businessRow(t, "880201")
	if row.status != "pending_approval" {
		t.Fatalf("an unapproved link turned back on is %q, want pending_approval", row.status)
	}
	if row.channelID != "bc-2" {
		t.Errorf("the row keeps connection id %q, want the new one bc-2 on the same row", row.channelID)
	}

	if _, err := c.channelStore().ApproveBusinessConnection(c.adminCtx(t), ids.MustParse(row.id)); err != nil {
		t.Fatalf("approving: %v", err)
	}
	c.deliver(t, sub, update(6205, "bc-2", false))
	c.deliver(t, sub, update(6206, "bc-3", true))
	if row, _ = c.businessRow(t, "880201"); row.status != "connected" || !row.approved {
		t.Fatalf("an approved link turned back on is %q (approved=%v), want connected", row.status, row.approved)
	}
}

func TestOnlyAnAdminApprovesOrDisconnectsABusinessConnection(t *testing.T) {
	c := setupTelegramConnected(t)
	runner, sub := newTelegramWorker(t, c, compose.JobRunnerConfig{})
	startTelegramWorker(t, runner)
	c.deliver(t, sub, businessUpdate{updateID: 6301, connectionID: "bc-x", ownerID: 880301, firstName: "Di", enabled: true})
	row, _ := c.businessRow(t, "880301")
	id := ids.MustParse(row.id)
	store := c.channelStore()

	rep := c.strangerRepCtx(t, map[string]principal.ObjectGrant{"channel_connection": {Read: true}})
	if _, err := store.ListBusinessConnections(rep); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a rep's list = %v, want permission denied", err)
	}
	if _, err := store.ApproveBusinessConnection(rep, id); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a rep's approval = %v, want permission denied", err)
	}
	if err := store.DisconnectBusinessConnection(rep, id); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a rep's disconnect = %v, want permission denied", err)
	}
	if after, _ := c.businessRow(t, "880301"); after.status != "pending_approval" {
		t.Fatalf("a refused approval left the link %q", after.status)
	}

	var listed struct {
		Data []struct{ ID, Status, AccountID string }
	}
	if status := c.Call(t, http.MethodGet, "/v1/channel-connections/telegram-business", nil, nil, &listed); status != http.StatusOK {
		t.Fatalf("the admin's list → %d, want 200", status)
	}
	if len(listed.Data) != 1 || listed.Data[0].AccountID != "880301" || listed.Data[0].Status != "pending_approval" {
		t.Fatalf("list = %+v, want the one waiting account", listed.Data)
	}
	var approved struct{ Status string }
	if status := c.Call(t, http.MethodPost, "/v1/channel-connections/telegram-business/"+row.id+"/approve", nil, nil, &approved); status != http.StatusOK || approved.Status != "connected" {
		t.Fatalf("the admin's approval → %d %q, want 200 connected", status, approved.Status)
	}
	if n := c.connectionAudits(t, row.id); n != 2 {
		t.Errorf("%d audit rows after a create and an approval, want 2", n)
	}

	agent := apptest.PassportBearer(t, c.AppEnv, "business agent", "read", "write")
	if status := c.Call(t, http.MethodPost, "/v1/channel-connections/telegram-business/"+row.id+"/approve", nil, agent, nil); status == http.StatusOK {
		t.Errorf("an agent passport approved a business connection → %d", status)
	}
	if status := c.Call(t, http.MethodDelete, "/v1/channel-connections/telegram-business/"+row.id, nil, nil, nil); status != http.StatusNoContent {
		t.Fatalf("the admin's disconnect → %d, want 204", status)
	}
	if _, ok := c.businessRow(t, "880301"); ok {
		t.Error("a disconnected link is still live")
	}
	if _, err := store.ApproveBusinessConnection(c.adminCtx(t), id); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("approving a disconnected link = %v, want not found", err)
	}

	// The owner links the bot again: a new link, waiting for approval.
	c.deliver(t, sub, businessUpdate{updateID: 6302, connectionID: "bc-y", ownerID: 880301, firstName: "Di", enabled: true})
	if again, ok := c.businessRow(t, "880301"); !ok || again.status != "pending_approval" || again.id == row.id {
		t.Errorf("a link made after a disconnect = %+v, want a new pending row", again)
	}
}

func TestBusinessConnectionsLeaveTheBotConnectionsUntouched(t *testing.T) {
	c := setupTelegramConnected(t)
	runner, sub := newTelegramWorker(t, c, compose.JobRunnerConfig{})
	startTelegramWorker(t, runner)
	c.deliver(t, sub, businessUpdate{updateID: 6401, connectionID: "bc-z", ownerID: 880401, firstName: "Ed", enabled: true})
	row, _ := c.businessRow(t, "880401")
	if _, err := c.channelStore().ApproveBusinessConnection(c.adminCtx(t), ids.MustParse(row.id)); err != nil {
		t.Fatalf("approving: %v", err)
	}

	due, err := capture.DueChannelConnections(context.Background(), c.Pool, capture.ProviderTelegram)
	if err != nil {
		t.Fatalf("due scan: %v", err)
	}
	if len(due) != 1 || due[0].ID != c.conn.ID {
		t.Errorf("the poll scan selects %+v, want only the bot connection: a connected business link is not polled", due)
	}
	listed, err := c.channelStore().List(c.adminCtx(t))
	if err != nil {
		t.Fatalf("listing: %v", err)
	}
	if len(listed) != 1 || listed[0].ID != c.conn.ID {
		t.Errorf("the bot connection list holds %d rows, want the bot alone", len(listed))
	}
	if _, err := c.channelStore().Get(c.adminCtx(t), ids.MustParse(row.id)); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("reading a business link as a bot connection = %v, want not found", err)
	}
	if err := c.channelStore().Disconnect(c.adminCtx(t), ids.MustParse(row.id)); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("disconnecting a business link as a bot = %v, want not found: it holds no credential to destroy", err)
	}

	if err := c.channelStore().Disconnect(c.adminCtx(t), c.conn.ID); err != nil {
		t.Fatalf("disconnecting the bot: %v", err)
	}
	if _, ok := c.businessRow(t, "880401"); ok {
		t.Error("a business link outlived the bot it rides on")
	}
}

// A link has its own provider, so a binary that reads every `telegram` row as the
// one live bot never meets it. A child row of another provider is not a Telegram
// link either, and the business endpoints neither list nor touch it.
func TestTheBusinessEndpointsServeOnlyTelegramBusinessRows(t *testing.T) {
	c := setupTelegramConnected(t)
	runner, sub := newTelegramWorker(t, c, compose.JobRunnerConfig{})
	startTelegramWorker(t, runner)
	c.deliver(t, sub, businessUpdate{updateID: 6501, connectionID: "bc-w", ownerID: 880501, firstName: "Fay", enabled: true})
	if n := c.count(t, `SELECT count(*) FROM channel_connection WHERE provider = 'telegram' AND archived_at IS NULL`); n != 1 {
		t.Fatalf("%d live telegram rows, want the bot alone: a link is a telegram_business row", n)
	}

	var foreign string
	if err := c.Owner.QueryRow(context.Background(), `
		INSERT INTO channel_connection
		  (provider, channel_id, channel_label, credential_ref, status, connected_by, account_id, parent_connection_id)
		SELECT 'whatsapp', 'wa-child', 'Other', '', 'pending_approval', connected_by, '990001', id
		  FROM channel_connection WHERE id = $1
		RETURNING id::text`, c.conn.ID).Scan(&foreign); err != nil {
		t.Fatalf("seeding a child row of another provider: %v", err)
	}
	foreignID := ids.MustParse(foreign)
	store := c.channelStore()

	listed, err := store.ListBusinessConnections(c.adminCtx(t))
	if err != nil {
		t.Fatalf("listing: %v", err)
	}
	if len(listed) != 1 || listed[0].AccountID != "880501" {
		t.Errorf("the Telegram list holds %+v, want the Telegram link alone", listed)
	}
	if _, err := store.ApproveBusinessConnection(c.adminCtx(t), foreignID); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("approving another provider's child row = %v, want not found", err)
	}
	if err := store.DisconnectBusinessConnection(c.adminCtx(t), foreignID); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("disconnecting another provider's child row = %v, want not found", err)
	}
	if err := store.Disconnect(c.adminCtx(t), c.conn.ID); err != nil {
		t.Fatalf("disconnecting the bot: %v", err)
	}
	if n := c.count(t, `SELECT count(*) FROM channel_connection WHERE id = $1 AND archived_at IS NULL AND status = 'pending_approval'`, foreign); n != 1 {
		t.Errorf("the other provider's child row was changed by Telegram operations")
	}
}
