// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// Mail addressed only to a colleague is the colleague's next step. A record's
// Just in line shows it to that colleague alone, as the Worklist does. Mail
// addressed to no seat and a chat message stay on the record for every reader.
func TestJustInShowsOnlyTheReadersOwnUnansweredMail(t *testing.T) {
	e := integration.Setup(t)
	at := time.Date(2026, 10, 21, 8, 0, 0, 0, time.UTC)
	addressed := connectSeatMailboxes(t, e, e.Rep1, e.AdminUser)[e.Rep1]
	// The record is the admin's, so only the address names the colleague.
	contact := e.SeedContact(t, "Colleague's buyer", &e.AdminUser)
	inbound := func(kind, thread string, to []string) ids.UUID {
		t.Helper()
		subject, body, direction := "Please send the specification", "Please send the specification for review.", "inbound"
		in := activities.LogActivityInput{
			Kind: kind, Subject: &subject, Body: &body, Direction: &direction, OccurredAt: &at, Source: "manual",
			ThreadKey: thread, Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}},
		}
		if kind == "email" {
			in.EmailFrom, in.EmailTo = "buyer@customer.test", to
		} else {
			in.ChannelProvider = "whatsapp"
		}
		logged, _, err := e.Activities.LogActivity(e.Admin(), in)
		if err != nil {
			t.Fatal(err)
		}
		return ids.UUID(logged.Id)
	}
	colleagues := inbound("email", "just-in-colleague", []string{addressed})
	unaddressed := inbound("email", "just-in-unaddressed", nil)
	chat := inbound("message", "just-in-chat", nil)
	classifier := principal.WithActor(e.Admin(), principal.Principal{Type: principal.PrincipalSystem, ID: "system:owed_verdict", Permissions: principal.Permissions{RowScope: principal.RowScopeAll}})
	for _, mail := range []ids.UUID{colleagues, unaddressed} {
		if _, err := e.Activities.SetOwedVerdict(classifier, mail, activities.OwedVerdictAsksUs, "fixture", at); err != nil {
			t.Fatal(err)
		}
	}
	later := func() time.Time { return at.Add(time.Hour) }
	store := e.Activities.WithClock(later)
	feed := newAttentionService(e.Pool, approvals.NewService(e.DB()), later)
	for reader, own := range map[ids.UUID]bool{e.Rep1: true, e.AdminUser: false} {
		ctx := e.As(reader, []ids.UUID{e.Team1}, integration.AdminPerms)
		justIn, err := store.LatestInbound(ctx, "contact", contact)
		if err != nil {
			t.Fatal(err)
		}
		shown := func(id ids.UUID) bool {
			return slices.ContainsFunc(justIn.Data, func(item crmcontracts.LatestInbound) bool { return ids.UUID(item.Activity.Id) == id })
		}
		page, err := feed.Worklist(ctx, "mine", "all", ids.Nil, 100, "")
		if err != nil {
			t.Fatal(err)
		}
		onWorklist := slices.ContainsFunc(page.Queue, func(row crmcontracts.WorklistItem) bool { return row.Id == colleagues.String() })
		if shown(colleagues) != own || onWorklist != own {
			t.Fatalf("reader %s: the colleague's mail on Just in %v, on the Worklist %v, want both %v", reader, shown(colleagues), onWorklist, own)
		}
		if !shown(unaddressed) || !shown(chat) {
			t.Fatalf("reader %s: unaddressed mail on Just in %v, chat message %v, want both", reader, shown(unaddressed), shown(chat))
		}
	}
}

// connectSeatMailboxes gives each seat a connected mailbox at its own address,
// so mail sent there names that seat, and answers the addresses.
func connectSeatMailboxes(t *testing.T, e *integration.Env, seats ...ids.UUID) map[ids.UUID]string {
	t.Helper()
	addresses := map[ids.UUID]string{}
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		for _, seat := range seats {
			var address string
			if err := tx.QueryRow(context.Background(), `SELECT email FROM app_user WHERE id=$1`, seat).Scan(&address); err != nil {
				return err
			}
			addresses[seat] = address
			if _, err := tx.Exec(context.Background(), `INSERT INTO capture_connection (user_id, provider, mail_posture, account_label) VALUES ($1,'gmail','shared',$2)`, seat, address); err != nil {
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
