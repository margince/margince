// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package channels

// A business account's seat has its own door for visibility, and the
// shared-mailbox routes never reach it. A link approved before seats existed
// waits for approval again and stores nothing until it has a seat.

import (
	"context"
	"net/http"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const seatlessLinkMigration = "../../../../migrations/core/1795639497_a_business_link_without_a_seat_waits_for_approval.up.sql"

func TestTheSharedMailboxRoutesAnswerNotFoundForAChatSeat(t *testing.T) {
	c := setupTelegramConnected(t)
	c.linkOnSeat(t, 880801, "bc-door", "connected", "held")
	seat := c.seats(t)[0].id

	for _, route := range []struct {
		method, path string
		body         any
	}{
		{http.MethodPatch, "/v1/shared-mailboxes/" + seat, integration.AnyMap{"mail_visibility": "held", "confirm": true}},
		{http.MethodPost, "/v1/shared-mailboxes/" + seat + "/disconnect", nil},
	} {
		if status := c.Call(t, route.method, route.path, route.body, nil, nil); status != http.StatusNotFound {
			t.Errorf("%s %s on a chat seat → %d, want 404", route.method, route.path, status)
		}
	}
	var listed struct{ Data []struct{ ID string } }
	if status := c.Call(t, http.MethodGet, "/v1/shared-mailboxes", nil, nil, &listed); status != http.StatusOK {
		t.Fatalf("listing the shared mailboxes → %d, want 200", status)
	}
	for _, box := range listed.Data {
		if box.ID == seat {
			t.Errorf("the shared-mailbox list holds the chat seat %s", seat)
		}
	}
	if got := c.seats(t)[0]; got.status != "connected" || got.posture != "held" {
		t.Errorf("the refused routes left the seat %+v, want it connected and held", got)
	}
}

func TestTheBusinessVisibilityRouteChangesTheSeatAndTheNextMessageFollowsIt(t *testing.T) {
	c := setupTelegramConnected(t)
	runner, sub := newTelegramWorker(t, c, compose.JobRunnerConfig{})
	startTelegramWorker(t, runner)
	c.linkOnSeat(t, 880802, "bc-visible", "connected", "held")
	row, _ := c.businessRow(t, "880802")
	path := "/v1/channel-connections/telegram-business/" + row.id + "/visibility"

	if status := c.Call(t, http.MethodPatch, path, integration.AnyMap{"mail_visibility": "shared"}, nil, nil); status != http.StatusUnprocessableEntity {
		t.Errorf("a change without confirm → %d, want 422", status)
	}
	if status := c.Call(t, http.MethodPatch, path, integration.AnyMap{"mail_visibility": "classified", "confirm": true}, nil, nil); status != http.StatusUnprocessableEntity {
		t.Errorf("classified → %d, want 422", status)
	}
	if status := c.Call(t, http.MethodPatch, path, integration.AnyMap{"mail_visibility": "shared", "confirm": true}, nil, nil); status != http.StatusUnprocessableEntity {
		t.Fatalf("shared without the workspace opt-in → %d, want 422, as for a mailbox", status)
	}
	if got := c.seats(t)[0].posture; got != "held" {
		t.Fatalf("refused changes left the seat %s, want held", got)
	}

	on := true
	settings := capture.NewSettings(compose.NewSettingsStore(c.Pool), compose.StoredConfig(c.Pool))
	actor, _ := principal.Actor(c.adminCtx(t))
	actor.Permissions = integration.AdminPerms
	if _, err := settings.Update(principal.WithActor(c.adminCtx(t), actor), capture.SettingsPatch{SharedPostureAllowed: &on}); err != nil {
		t.Fatalf("opting the workspace in: %v", err)
	}
	var changed struct{ MailVisibility string }
	if status := c.Call(t, http.MethodPatch, path, integration.AnyMap{"mail_visibility": "shared", "confirm": true}, nil, &changed); status != http.StatusOK {
		t.Fatalf("shared with the opt-in → %d, want 200", status)
	}
	if changed.MailVisibility != "shared" || c.seats(t)[0].posture != "shared" {
		t.Fatalf("the route answered %q and the seat is %q, want shared both", changed.MailVisibility, c.seats(t)[0].posture)
	}

	m := businessMessage{updateID: 7901, messageID: 31, chatID: 990802, fromID: 990802, connection: "bc-visible", text: "price?"}
	c.deliverMessage(t, sub, m)
	awaitJobKind(t, sub, compose.TelegramIngestArgs{}.Kind())
	if filed, ok := c.filed(t, m.sourceID(880802)); !ok || filed.audience != "workspace" {
		t.Errorf("the next message is filed as %+v (%v), want audience workspace", filed, ok)
	}
}

func TestALinkApprovedBeforeSeatsWaitsForApprovalAndFilesAgainOnceItHasASeat(t *testing.T) {
	c := setupTelegramConnected(t)
	runner, sub := newTelegramWorker(t, c, compose.JobRunnerConfig{})
	startTelegramWorker(t, runner)
	if err := apptest.InWorkspace(c.AppEnv, t, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			INSERT INTO channel_connection
			  (provider, channel_id, channel_label, credential_ref, status, connected_by, account_id,
			   parent_connection_id, ownership, approved_at)
			VALUES ('telegram_business', 'bc-old', 'Mia', '', 'connected', $1, '880803', $2, 'workspace', now())`,
			c.admin, c.conn.ID)
		return err
	}); err != nil {
		t.Fatalf("writing the link approved before seats: %v", err)
	}

	first := businessMessage{updateID: 8001, messageID: 1, chatID: 990803, fromID: 990803, connection: "bc-old", text: "hello"}
	c.deliverMessage(t, sub, first)
	c.assertNothingStored(t, first)

	migration, err := os.ReadFile(seatlessLinkMigration)
	if err != nil {
		t.Fatalf("reading the migration: %v", err)
	}
	if err := apptest.InWorkspace(c.AppEnv, t, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), string(migration))
		return err
	}); err != nil {
		t.Fatalf("running the migration: %v", err)
	}
	row, _ := c.businessRow(t, "880803")
	if row.status != "pending_approval" || row.approved {
		t.Fatalf("the link after the migration is %+v, want pending_approval and not approved", row)
	}

	team := c.readerTeam(t)
	approve := "/v1/channel-connections/telegram-business/" + row.id + "/approve"
	if status := c.Call(t, http.MethodPost, approve, integration.AnyMap{"readerTeamId": team.String()}, nil, nil); status != http.StatusOK {
		t.Fatalf("re-approving with a reader team → %d, want 200", status)
	}
	second := businessMessage{updateID: 8002, messageID: 2, chatID: 990803, fromID: 990803, connection: "bc-old", text: "hello again"}
	c.deliverMessage(t, sub, second)
	awaitJobKind(t, sub, compose.TelegramIngestArgs{}.Kind())
	if _, ok := c.filed(t, second.sourceID(880803)); !ok {
		t.Error("a message on the re-approved link was not filed")
	}
}
