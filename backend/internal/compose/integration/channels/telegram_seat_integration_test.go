// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package channels

// The seat an approved Telegram Business account files through. Approval gives
// the account one seat, a disconnect ends it, and a message on the account reads
// its posture from it. The admin side runs over the real HTTP surface, the
// message side over the real poll, ingest worker and sink.

import (
	"context"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// readerTeam makes a team the connecting admin belongs to.
func (c *telegramEnv) readerTeam(t *testing.T) ids.UUID {
	t.Helper()
	team := ids.NewV7()
	if err := apptest.InWorkspace(c.AppEnv, t, func(tx pgx.Tx) error {
		if _, err := tx.Exec(context.Background(), `INSERT INTO team (id, name) VALUES ($1, $2)`, team, "Front desk "+team.String()); err != nil {
			return err
		}
		_, err := tx.Exec(context.Background(), `INSERT INTO team_membership (team_id, user_id) VALUES ($1, $2::uuid)`, team, c.admin)
		return err
	}); err != nil {
		t.Fatalf("making the reader team: %v", err)
	}
	return team
}

type seatRow struct {
	id, status, posture, label string
	team                       *string
	user                       *string
}

func (c *telegramEnv) seats(t *testing.T) []seatRow {
	t.Helper()
	var out []seatRow
	if err := apptest.InWorkspace(c.AppEnv, t, func(tx pgx.Tx) error {
		rows, err := tx.Query(context.Background(), `
			SELECT id::text, status, mail_posture, account_label, reader_team_id::text, user_id::text
			  FROM capture_connection WHERE provider = 'telegram_business' ORDER BY created_at`)
		if err != nil {
			return err
		}
		out, err = pgx.CollectRows(rows, func(r pgx.CollectableRow) (seatRow, error) {
			var s seatRow
			err := r.Scan(&s.id, &s.status, &s.posture, &s.label, &s.team, &s.user)
			return s, err
		})
		return err
	}); err != nil {
		t.Fatalf("reading the seats: %v", err)
	}
	return out
}

func TestApprovingALinkMakesOneHeldTeamSeatAndAnotherApprovalMakesNoSecond(t *testing.T) {
	c := setupTelegramConnected(t)
	runner, sub := newTelegramWorker(t, c, compose.JobRunnerConfig{})
	startTelegramWorker(t, runner)
	c.deliver(t, sub, businessUpdate{updateID: 6501, connectionID: "bc-seat", ownerID: 880501, firstName: "Fay", enabled: true})
	row, _ := c.businessRow(t, "880501")
	if got := c.seats(t); len(got) != 0 {
		t.Fatalf("%d seats before any approval, want none", len(got))
	}
	path := "/v1/channel-connections/telegram-business/" + row.id + "/approve"
	var noTeam struct{ Code string }
	if status := c.Call(t, http.MethodPost, path, nil, nil, &noTeam); status != http.StatusUnprocessableEntity {
		t.Fatalf("approving a new account with no reader team → %d, want 422", status)
	}
	if got := c.seats(t); len(got) != 0 {
		t.Fatalf("a refused approval left %d seats", len(got))
	}

	team := c.readerTeam(t)
	var approved struct {
		Status, MailVisibility string
		SeatID, ReaderTeamID   string
	}
	if status := c.Call(t, http.MethodPost, path, integration.AnyMap{"readerTeamId": team.String()}, nil, &approved); status != http.StatusOK {
		t.Fatalf("approving with a reader team → %d, want 200", status)
	}
	seats := c.seats(t)
	if len(seats) != 1 {
		t.Fatalf("%d seats after the approval, want 1", len(seats))
	}
	seat := seats[0]
	if seat.status != "connected" || seat.posture != "held" || seat.label != "880501" || seat.user != nil || seat.team == nil || *seat.team != team.String() {
		t.Errorf("seat = %+v, want a connected team seat for 880501 read by %s and held", seat, team)
	}
	if approved.SeatID != seat.id || approved.ReaderTeamID != team.String() || approved.MailVisibility != "held" {
		t.Errorf("the approval answered %+v, want the seat, its team and held", approved)
	}

	if status := c.Call(t, http.MethodPost, path, nil, nil, nil); status != http.StatusOK {
		t.Fatalf("approving again → %d, want 200", status)
	}
	if status := c.Call(t, http.MethodPost, path, integration.AnyMap{"readerTeamId": c.readerTeam(t).String()}, nil, nil); status != http.StatusConflict {
		t.Fatalf("approving again naming another team → %d, want 409", status)
	}
	if got := c.seats(t); len(got) != 1 || got[0].id != seat.id {
		t.Fatalf("seats after a second approval = %+v, want the same one", got)
	}
}

func TestDisconnectingALinkEndsItsSeatAndALaterApprovalReusesIt(t *testing.T) {
	c := setupTelegramConnected(t)
	runner, sub := newTelegramWorker(t, c, compose.JobRunnerConfig{})
	startTelegramWorker(t, runner)
	c.deliver(t, sub, businessUpdate{updateID: 6601, connectionID: "bc-end", ownerID: 880601, firstName: "Gus", enabled: true})
	row, _ := c.businessRow(t, "880601")
	team := c.readerTeam(t)
	approve := func(id string, body map[string]any) {
		t.Helper()
		if status := c.Call(t, http.MethodPost, "/v1/channel-connections/telegram-business/"+id+"/approve", body, nil, nil); status != http.StatusOK {
			t.Fatalf("approving %s → %d, want 200", id, status)
		}
	}
	approve(row.id, integration.AnyMap{"readerTeamId": team.String()})
	seat := c.seats(t)[0]

	if status := c.Call(t, http.MethodDelete, "/v1/channel-connections/telegram-business/"+row.id, nil, nil, nil); status != http.StatusNoContent {
		t.Fatalf("disconnecting → %d, want 204", status)
	}
	if got := c.seats(t); len(got) != 1 || got[0].id != seat.id || got[0].status != "disconnected" || got[0].team == nil || *got[0].team != team.String() {
		t.Fatalf("seats after a disconnect = %+v, want the same seat disconnected with its readers", got)
	}

	c.deliver(t, sub, businessUpdate{updateID: 6602, connectionID: "bc-end-2", ownerID: 880601, firstName: "Gus", enabled: true})
	again, _ := c.businessRow(t, "880601")
	approve(again.id, nil)
	if got := c.seats(t); len(got) != 1 || got[0].id != seat.id || got[0].status != "connected" {
		t.Fatalf("seats after a reconnect = %+v, want seat %s connected again", got, seat.id)
	}
}

func TestAMessageOnAHeldAccountStaysFromAnOutsiderAndASharedAccountOpensIt(t *testing.T) {
	for _, tc := range []struct{ posture, audience string }{{"held", "participants"}, {"shared", "workspace"}} {
		t.Run(tc.posture, func(t *testing.T) {
			c := setupTelegramConnected(t)
			runner, sub := newTelegramWorker(t, c, compose.JobRunnerConfig{})
			startTelegramWorker(t, runner)
			c.linkOnSeat(t, 880701, "bc-posture", "connected", tc.posture)
			m := businessMessage{updateID: 7801, messageID: 21, chatID: 990701, fromID: 990701, connection: "bc-posture", text: "price?"}

			c.deliverMessage(t, sub, m)
			awaitJobKind(t, sub, compose.TelegramIngestArgs{}.Kind())

			filed, ok := c.filed(t, m.sourceID(880701))
			if !ok || filed.audience != tc.audience {
				t.Fatalf("a message on a %s account is filed as %+v (%v), want audience %s", tc.posture, filed, ok, tc.audience)
			}
			seat := c.seats(t)[0]
			if n := c.count(t, `SELECT count(*) FROM capture_import WHERE activity_id = $1 AND connection_id = $2 AND posture_at_import = $3`,
				filed.id, seat.id, tc.posture); n != 1 {
				t.Errorf("%d import rows from the seat under %s, want 1", n, tc.posture)
			}
		})
	}
}
