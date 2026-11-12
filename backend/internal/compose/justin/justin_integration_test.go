// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package justin_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// justInFixture files inbound mail on records the admin owns. Rep1 and the
// admin have connected mailboxes; Rep2 is known only by its login address.
type justInFixture struct {
	e       *integration.Env
	at      time.Time
	store   *activities.Store
	address map[ids.UUID]string
}

func newJustInFixture(t *testing.T) *justInFixture {
	t.Helper()
	e := integration.Setup(t)
	at := time.Date(2026, 10, 21, 8, 0, 0, 0, time.UTC)
	address := connectSeatMailboxes(t, e, e.Rep1, e.AdminUser)
	address[e.Rep2] = seatLogins(t, e, e.Rep2)[e.Rep2]
	return &justInFixture{e: e, at: at, store: e.Activities.WithClock(func() time.Time { return at.Add(time.Hour) }), address: address}
}

// record is a new contact the admin owns, so only an address names a seat.
func (f *justInFixture) record(t *testing.T) ids.UUID {
	t.Helper()
	return f.e.SeedContact(t, "Buyer "+ids.NewV7().String(), &f.e.AdminUser)
}

func (f *justInFixture) reader(seat ids.UUID) context.Context {
	return f.e.As(seat, []ids.UUID{f.e.Team1}, integration.AdminPerms)
}

// inbound logs their mail, or with kind "message" a chat message, on the
// record. Mail is judged to ask us something, so it waits for a reply.
func (f *justInFixture) inbound(t *testing.T, contact ids.UUID, kind string, at time.Time, to, cc []ids.UUID) ids.UUID {
	t.Helper()
	subject, body, direction := "Please send the specification", "Please send the specification for review.", "inbound"
	in := activities.LogActivityInput{
		Kind: kind, Subject: &subject, Body: &body, Direction: &direction, OccurredAt: &at, Source: "manual",
		ThreadKey: "just-in-" + ids.NewV7().String(), Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}},
	}
	if kind != "email" {
		in.ChannelProvider = "whatsapp"
	} else {
		in.EmailFrom = "buyer@customer.test"
		for _, seat := range to {
			in.EmailTo = append(in.EmailTo, f.address[seat])
		}
		for _, seat := range cc {
			in.EmailCc = append(in.EmailCc, f.address[seat])
		}
	}
	logged, _, err := f.e.Activities.LogActivity(f.e.Admin(), in)
	if err != nil {
		t.Fatal(err)
	}
	id := ids.UUID(logged.Id)
	if kind == "email" {
		classifier := principal.WithActor(f.e.Admin(), principal.Principal{Type: principal.PrincipalSystem, ID: "system:owed_verdict", Permissions: principal.Permissions{RowScope: principal.RowScopeAll}})
		if _, err := f.e.Activities.SetOwedVerdict(classifier, id, activities.OwedVerdictAsksUs, "fixture", at); err != nil {
			t.Fatal(err)
		}
	}
	return id
}

// justIn is what the record's Just in line lists for one reader.
func (f *justInFixture) justIn(t *testing.T, seat, contact ids.UUID) []ids.UUID {
	t.Helper()
	list, err := f.store.LatestInbound(f.reader(seat), "contact", contact)
	if err != nil {
		t.Fatal(err)
	}
	out := []ids.UUID{}
	for _, item := range list.Data {
		out = append(out, ids.UUID(item.Activity.Id))
	}
	return out
}

// Mail addressed only to a colleague is the colleague's next step. A record's
// Just in line shows it to that colleague alone, as the Worklist does. Mail
// addressed to no seat and a chat message stay on the record for every reader.
func TestJustInShowsOnlyTheReadersOwnUnansweredMail(t *testing.T) {
	f := newJustInFixture(t)
	contact := f.record(t)
	repsMail := f.inbound(t, contact, "email", f.at, []ids.UUID{f.e.Rep1}, nil)
	unaddressed := f.inbound(t, contact, "email", f.at, nil, nil)
	chat := f.inbound(t, contact, "message", f.at, nil, nil)
	feed := compose.NewAttentionService(f.e.Pool, approvals.NewService(f.e.DB()), func() time.Time { return f.at.Add(time.Hour) })
	for reader, own := range map[ids.UUID]bool{f.e.Rep1: true, f.e.AdminUser: false} {
		shown := f.justIn(t, reader, contact)
		page, err := feed.Worklist(f.reader(reader), "mine", "all", ids.Nil, 100, "")
		if err != nil {
			t.Fatal(err)
		}
		onWorklist := slices.ContainsFunc(page.Queue, func(row crmcontracts.WorklistItem) bool { return row.Id == repsMail.String() })
		if slices.Contains(shown, repsMail) != own || onWorklist != own {
			t.Fatalf("reader %s: Rep1's mail on Just in %v, on the Worklist %v, want both %v", reader, slices.Contains(shown, repsMail), onWorklist, own)
		}
		if !slices.Contains(shown, unaddressed) || !slices.Contains(shown, chat) {
			t.Fatalf("reader %s: unaddressed mail on Just in %v, chat message %v, want both", reader, slices.Contains(shown, unaddressed), slices.Contains(shown, chat))
		}
	}
}

// A mail is the one seat's in its To line. Two seats there make it nobody's,
// so both see it. A seat only in Cc does not make it theirs.
func TestJustInNamesTheSoleSeatInTheToLine(t *testing.T) {
	f := newJustInFixture(t)
	rep, admin, unconnected := f.e.Rep1, f.e.AdminUser, f.e.Rep2
	for _, tc := range []struct {
		name   string
		to, cc []ids.UUID
		shown  map[ids.UUID]bool
	}{
		{"two seats in To", []ids.UUID{rep, admin}, nil, map[ids.UUID]bool{rep: true, admin: true}},
		{"reader in To, colleague in Cc", []ids.UUID{rep}, []ids.UUID{admin}, map[ids.UUID]bool{rep: true, admin: false}},
		{"reader in Cc, colleague in To", []ids.UUID{admin}, []ids.UUID{rep}, map[ids.UUID]bool{rep: false, admin: true}},
		{"a seat with no connected mailbox in To", []ids.UUID{unconnected}, nil, map[ids.UUID]bool{rep: false, admin: false, unconnected: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			contact := f.record(t)
			mail := f.inbound(t, contact, "email", f.at, tc.to, tc.cc)
			for reader, want := range tc.shown {
				if got := slices.Contains(f.justIn(t, reader, contact), mail); got != want {
					t.Errorf("reader %s sees the mail on Just in = %v, want %v", reader, got, want)
				}
			}
		})
	}
}

// The record's Write button replies to the newest waiting thread it is handed.
// A colleague's newer mail must not take that place from the reader's own,
// which a filter applied after the limit would let it do.
func TestTheWriteButtonRepliesToTheReadersOwnWaitingThread(t *testing.T) {
	f := newJustInFixture(t)
	contact := f.record(t)
	own := f.inbound(t, contact, "email", f.at, []ids.UUID{f.e.Rep1}, nil)
	colleagues := f.inbound(t, contact, "email", f.at.Add(30*time.Minute), []ids.UUID{f.e.AdminUser}, nil)
	entity, limit := "contact", 1
	for reader, want := range map[ids.UUID]ids.UUID{f.e.Rep1: own, f.e.AdminUser: colleagues} {
		rows, _, err := f.store.ListActivities(f.reader(reader), activities.ListActivitiesInput{
			EntityType: &entity, EntityID: &contact, WaitingReply: true, Limit: &limit,
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 1 || ids.UUID(rows[0].Id) != want {
			t.Fatalf("reader %s: the Write button's pick holds %d rows, want only their own thread %s", reader, len(rows), want)
		}
	}
}

// seatLogins answers each seat's login address, which names the seat on mail
// whether or not a mailbox is connected.
func seatLogins(t *testing.T, e *integration.Env, seats ...ids.UUID) map[ids.UUID]string {
	t.Helper()
	addresses := map[ids.UUID]string{}
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		for _, seat := range seats {
			var address string
			if err := tx.QueryRow(context.Background(), `SELECT email FROM app_user WHERE id=$1`, seat).Scan(&address); err != nil {
				return err
			}
			addresses[seat] = address
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return addresses
}

// connectSeatMailboxes gives each seat a connected mailbox at its login
// address, so mail sent there names that seat, and answers the addresses.
func connectSeatMailboxes(t *testing.T, e *integration.Env, seats ...ids.UUID) map[ids.UUID]string {
	t.Helper()
	addresses := seatLogins(t, e, seats...)
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		for _, seat := range seats {
			if _, err := tx.Exec(context.Background(), `INSERT INTO capture_connection (user_id, provider, mail_posture, account_label) VALUES ($1,'gmail','shared',$2)`, seat, addresses[seat]); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return addresses
}
