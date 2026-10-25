// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

// Which colleagues are on a contact's threads without a connected mailbox.
//
// Against real Postgres because the answer is one statement over six tables.
// The participant rows come from StampFurtherParticipants, the writer capture
// uses, so the test sees the user_id binding rule as production does.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

var testMailProviders = []string{"gmail", "graph", "imap"}

type seatThread struct {
	kind      string
	audience  string
	attested  bool
	archived  bool
	seatEmail string
	role      string
}

// seedSeat inserts a colleague with the given status and a unique login.
func seedSeat(ctx context.Context, t *testing.T, tx pgx.Tx, status string) (ids.UUID, string) {
	t.Helper()
	id := ids.NewV7()
	email := "seat-" + id.String() + "@example.test"
	if _, err := tx.Exec(ctx,
		`INSERT INTO app_user (id, email, display_name, status) VALUES ($1, $2, $2, $3)`,
		id, email, status); err != nil {
		t.Fatalf("seeding seat: %v", err)
	}
	return id, email
}

func connectSeat(ctx context.Context, t *testing.T, tx pgx.Tx, userID ids.UUID, provider string) {
	t.Helper()
	if _, err := tx.Exec(ctx, `
		INSERT INTO capture_connection (provider, user_id, status, auth)
		VALUES ($1, $2, 'connected', '\x00')`, provider, userID); err != nil {
		t.Fatalf("connecting %s: %v", provider, err)
	}
}

// seedThread writes a contact's message or meeting naming the seat's address
// as a recipient, through StampFurtherParticipants.
func seedThread(ctx context.Context, t *testing.T, tx pgx.Tx, th seatThread) ids.UUID {
	t.Helper()
	contactID := ids.NewV7()
	if _, err := tx.Exec(ctx, `
		INSERT INTO contact (id, full_name, source, captured_by)
		VALUES ($1, 'Outside Contact', 'manual', 'human:test')`, contactID); err != nil {
		t.Fatalf("seeding contact: %v", err)
	}
	var activityID ids.ActivityID
	archivedAt := "NULL"
	if th.archived {
		archivedAt = "now()"
	}
	if err := tx.QueryRow(ctx, `
		INSERT INTO activity (kind, occurred_at, direction, source, captured_by, audience, archived_at)
		VALUES ($1, now(), 'inbound', 'imap', 'connector:imap', $2, `+archivedAt+`)
		RETURNING id`, th.kind, th.audience).Scan(&activityID); err != nil {
		t.Fatalf("seeding activity: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO activity_participant (activity_id, contact_id, role)
		VALUES ($1, $2, 'from')`, activityID, contactID); err != nil {
		t.Fatalf("seeding contact participant: %v", err)
	}
	role := th.role
	if role == "" {
		role = "to"
	}
	if err := StampFurtherParticipants(ctx, tx, activityID, th.kind, "", th.attested,
		[]connector.MessageParticipant{{Email: th.seatEmail, Role: role}}); err != nil {
		t.Fatalf("stamping participants: %v", err)
	}
	return contactID
}

func seatGaps(ctx context.Context, t *testing.T, tx pgx.Tx, contactID ids.UUID) []UncapturedSeat {
	t.Helper()
	got, err := SeatsWithoutMailbox(ctx, tx, contactID, testMailProviders, 10)
	if err != nil {
		t.Fatalf("SeatsWithoutMailbox: %v", err)
	}
	return got.Seats
}

func TestSeatCoverage(t *testing.T) {
	cases := []struct {
		name       string
		status     string
		thread     seatThread
		connect    string
		deactivate bool
		seatLater  bool
		wantSeat   bool
		wantInvite bool
	}{
		{name: "an invited seat on an unattested Cc line", status: "invited",
			thread: seatThread{kind: "email", audience: "workspace"}, wantSeat: true, wantInvite: true},
		{name: "an active seat with no mailbox is a permitted positive", status: "active",
			thread: seatThread{kind: "email", audience: "workspace"}, wantSeat: true},
		{name: "an invited organiser of an attested meeting is found by user_id", status: "invited",
			thread: seatThread{kind: "meeting", audience: "workspace", attested: true}, wantSeat: true, wantInvite: true},
		{name: "a seat with a connected mailbox is covered", status: "active",
			thread: seatThread{kind: "email", audience: "workspace"}, connect: "gmail"},
		{name: "a seat with only a calendar connection still has no mailbox", status: "active",
			thread: seatThread{kind: "email", audience: "workspace"}, connect: "gcal", wantSeat: true},
		{name: "a limited message discloses nothing", status: "active",
			thread: seatThread{kind: "email", audience: "participants"}},
		{name: "an archived activity discloses nothing", status: "active",
			thread: seatThread{kind: "email", audience: "workspace", archived: true}},
		{name: "an address on a Bcc line is not matched to a seat", status: "active",
			thread: seatThread{kind: "email", audience: "workspace", role: "bcc"}},
		{name: "a seat invited after the thread was captured is not on it", status: "active", seatLater: true,
			thread: seatThread{kind: "email", audience: "workspace"}},
		{name: "a deactivated seat is not noticed", status: "active", deactivate: true,
			thread: seatThread{kind: "email", audience: "workspace"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, pool := meetingTenureEnv(t)
			if err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
				seat, email := seedSeat(ctx, t, tx, tc.status)
				if tc.connect != "" {
					connectSeat(ctx, t, tx, seat, tc.connect)
				}
				if tc.deactivate {
					if _, err := tx.Exec(ctx, `UPDATE app_user SET status = 'deactivated' WHERE id = $1`, seat); err != nil {
						return err
					}
				}
				if tc.seatLater {
					if _, err := tx.Exec(ctx, `UPDATE app_user SET created_at = now() + interval '1 hour' WHERE id = $1`, seat); err != nil {
						return err
					}
				}
				tc.thread.seatEmail = email
				contactID := seedThread(ctx, t, tx, tc.thread)

				got := seatGaps(ctx, t, tx, contactID)
				if !tc.wantSeat {
					if len(got) != 0 {
						t.Fatalf("got %v, want no seat", got)
					}
					return nil
				}
				if len(got) != 1 || got[0].UserID != seat || got[0].Invited != tc.wantInvite {
					t.Fatalf("got %+v, want seat %v invited=%v", got, seat, tc.wantInvite)
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// An unattested Cc line is the sender's text, so it is reported but never
// bound to a user_id and never becomes an interaction edge.
func TestSeatCoverageDoesNotBindAnUnattestedAddress(t *testing.T) {
	ctx, pool := meetingTenureEnv(t)
	if err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		seat, email := seedSeat(ctx, t, tx, "invited")
		contactID := seedThread(ctx, t, tx, seatThread{kind: "email", audience: "workspace", seatEmail: email})
		if len(seatGaps(ctx, t, tx, contactID)) != 1 {
			t.Fatal("the invited seat was not noticed")
		}
		var bound, edges int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM activity_participant WHERE user_id = $1`, seat).Scan(&bound); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM graph_interaction_edge WHERE user_id = $1`, seat).Scan(&edges); err != nil {
			return err
		}
		if bound != 0 || edges != 0 {
			t.Errorf("an unattested address bound %d participant rows and %d edges to the seat", bound, edges)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
