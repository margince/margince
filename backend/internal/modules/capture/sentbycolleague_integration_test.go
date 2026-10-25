// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// colleagueCopy is the copy a seat's mailbox receives when a colleague answers
// a customer and copies the seat: From the colleague, To the customer.
func colleagueCopy(sourceID, from string) connector.NormalizedRecord {
	rec := mailRecord(sourceID, from, from, "dana@client.io", "rep@acme.com", "eli@client.io")
	rec.Participants = []connector.MessageParticipant{
		{Email: "dana@client.io", Role: connector.ParticipantRoleTo},
		{Email: "rep@acme.com", Role: connector.ParticipantRoleCC},
		{Email: "eli@client.io", Role: connector.ParticipantRoleCC},
	}
	// As Gmail delivers it: its receiving server passed DMARC for the From domain.
	rec.CapturedBy = "connector:gmail"
	rec.SenderAuthentication = connector.SenderAuthentication{Server: "mx.google.com", DMARCPassDomain: "acme.com"}
	return rec
}

type storedMail struct {
	direction, counterparty string
	attested                bool
	colleague               *ids.UUID
	fromUser                *ids.UUID
}

func storedMailFor(ctx context.Context, t *testing.T, db *database.DB, sourceID string) storedMail {
	t.Helper()
	var m storedMail
	if err := db.Tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT a.direction, coalesce(a.counterparty_email, ''), a.counterparty_outbound_attested, a.sent_by_colleague,
			       (SELECT p.user_id FROM activity_participant p
			         WHERE p.activity_id = a.id AND p.role = 'from' AND p.user_id IS NOT NULL)
			  FROM activity a WHERE a.source_id = $1`, sourceID).
			Scan(&m.direction, &m.counterparty, &m.attested, &m.colleague, &m.fromUser)
	}); err != nil {
		t.Fatalf("reading the captured mail: %v", err)
	}
	return m
}

func seedSeat(ctx context.Context, t *testing.T, db *database.DB, email, status string) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	if err := db.Tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO app_user (id, email, display_name, status) VALUES ($1, $2, $2, $3)`,
			id, email, status)
		return err
	}); err != nil {
		t.Fatalf("seeding %s: %v", email, err)
	}
	return id
}

// A colleague's answer to a customer is the workspace's own mail to that
// customer. It is outbound, to the first outside recipient. The colleague wrote
// it, not the seat whose mailbox held the copy. Nobody attested it.
func TestAColleaguesCopiedMailIsOurMailToTheCustomer(t *testing.T) {
	ctx, db := bootstrapInternalMailWorkspace(t, "acme.com")
	ws, _ := principal.WorkspaceID(ctx)
	seat := seedSeat(ctx, t, db, "rep@acme.com", "active")
	colleague := seedSeat(ctx, t, db, "sam@acme.com", "invited")

	if _, err := capture.NewSink(db).Upsert(capturedAs(ctx, ws, seat, colleagueCopy("colleague-1", "Sam@acme.com"))); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	got := storedMailFor(ctx, t, db, "colleague-1")
	if got.direction != connector.DirectionOutbound {
		t.Errorf("direction = %q, want outbound: the customer's question would stay owed", got.direction)
	}
	if got.counterparty != "dana@client.io" {
		t.Errorf("counterparty = %q, want the first outside recipient", got.counterparty)
	}
	if got.attested {
		t.Error("the copy is attested, but no provider vouched that this mailbox sent it")
	}
	if got.colleague == nil || *got.colleague != colleague {
		t.Errorf("sent_by_colleague = %v, want the colleague %v", got.colleague, colleague)
	}
	if got.fromUser == nil || *got.fromUser != colleague {
		t.Errorf("the From participant is %v, want the colleague %v and not the seat", got.fromUser, colleague)
	}
	var seatRole string
	if err := db.Tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT string_agg(p.role, ',') FROM activity_participant p
			JOIN activity a ON a.id = p.activity_id WHERE a.source_id = 'colleague-1' AND p.user_id = $1`, seat).Scan(&seatRole)
	}); err != nil {
		t.Fatal(err)
	}
	if seatRole != "to" {
		t.Errorf("the seat that held the copy is stamped %q, want only \"to\": it received the mail and did not write it", seatRole)
	}
}

// Only a seat's exact address is a colleague. An archived seat, or another
// address at the own domain, stays the inbound mail capture always filed.
func TestOnlyALiveSeatsAddressIsAColleague(t *testing.T) {
	ctx, db := bootstrapInternalMailWorkspace(t, "acme.com")
	ws, _ := principal.WorkspaceID(ctx)
	seat := seedSeat(ctx, t, db, "rep@acme.com", "active")
	gone := seedSeat(ctx, t, db, "gone@acme.com", "deactivated")
	if err := db.Tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE app_user SET archived_at = now() WHERE id = $1`, gone)
		return err
	}); err != nil {
		t.Fatal(err)
	}

	for _, from := range []string{"gone@acme.com", "sales@acme.com"} {
		t.Run(from, func(t *testing.T) {
			if _, err := capture.NewSink(db).Upsert(capturedAs(ctx, ws, seat, colleagueCopy("not-"+from, from))); err != nil {
				t.Fatalf("Upsert: %v", err)
			}
			got := storedMailFor(ctx, t, db, "not-"+from)
			if got.direction != connector.DirectionInbound || got.colleague != nil {
				t.Errorf("mail from %s was read as a colleague's (direction %q, colleague %v)", from, got.direction, got.colleague)
			}
		})
	}
}

// A second, outside From address is a co-author, not the customer the
// colleague wrote to. The counterparty is the first outside recipient.
func TestAColleaguesCoAuthorIsNotTheCustomer(t *testing.T) {
	ctx, db := bootstrapInternalMailWorkspace(t, "acme.com")
	ws, _ := principal.WorkspaceID(ctx)
	seat := seedSeat(ctx, t, db, "rep@acme.com", "active")
	seedSeat(ctx, t, db, "sam@acme.com", "invited")
	rec := colleagueCopy("co-authored-1", "sam@acme.com")
	rec.Addresses = []string{"sam@acme.com", "pat@agency.io", "dana@client.io", "rep@acme.com", "eli@client.io"}

	if _, err := capture.NewSink(db).Upsert(capturedAs(ctx, ws, seat, rec)); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	got := storedMailFor(ctx, t, db, "co-authored-1")
	if got.direction != connector.DirectionOutbound || got.counterparty != "dana@client.io" {
		t.Errorf("stored as %s to %q, want outbound to the first outside recipient dana@client.io", got.direction, got.counterparty)
	}
}

// A copy to a distribution list names more recipients than capture keeps as
// participants. They are withheld, and the first of them is still the customer.
func TestAColleaguesMailToAListIsStillOurs(t *testing.T) {
	ctx, db := bootstrapInternalMailWorkspace(t, "acme.com")
	ws, _ := principal.WorkspaceID(ctx)
	seat := seedSeat(ctx, t, db, "rep@acme.com", "active")
	seedSeat(ctx, t, db, "sam@acme.com", "invited")
	rec := colleagueCopy("list-1", "sam@acme.com")
	rec.Participants = nil
	for i := range connector.MaxParticipants + 1 {
		rec.WithheldParties = append(rec.WithheldParties,
			connector.MessageParticipant{Email: fmt.Sprintf("buyer%d@client.io", i), Role: connector.ParticipantRoleTo})
	}

	if _, err := capture.NewSink(db).Upsert(capturedAs(ctx, ws, seat, rec)); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	got := storedMailFor(ctx, t, db, "list-1")
	if got.direction != connector.DirectionOutbound || got.counterparty != "buyer0@client.io" {
		t.Errorf("stored as %s to %q, want outbound to the first withheld recipient buyer0@client.io", got.direction, got.counterparty)
	}
}

// A From the receiving server did not authenticate is the sender's word alone,
// and the copy stays the inbound mail capture always filed.
func TestAnUnauthenticatedColleagueIsNotTrusted(t *testing.T) {
	ctx, db := bootstrapInternalMailWorkspace(t, "acme.com")
	ws, _ := principal.WorkspaceID(ctx)
	seat := seedSeat(ctx, t, db, "rep@acme.com", "active")
	seedSeat(ctx, t, db, "sam@acme.com", "invited")

	for name, unauthenticate := range map[string]func(*connector.NormalizedRecord){
		"no result":                 func(r *connector.NormalizedRecord) { r.SenderAuthentication = connector.SenderAuthentication{} },
		"DMARC for another domain":  func(r *connector.NormalizedRecord) { r.SenderAuthentication.DMARCPassDomain = "elsewhere.example" },
		"a server other than Gmail": func(r *connector.NormalizedRecord) { r.SenderAuthentication.Server = "mx.elsewhere.example" },
		"over IMAP":                 func(r *connector.NormalizedRecord) { r.CapturedBy = "connector:imap" },
	} {
		t.Run(name, func(t *testing.T) {
			rec := colleagueCopy("unauth-"+name, "sam@acme.com")
			unauthenticate(&rec)
			if _, err := capture.NewSink(db).Upsert(capturedAs(ctx, ws, seat, rec)); err != nil {
				t.Fatalf("Upsert: %v", err)
			}
			got := storedMailFor(ctx, t, db, "unauth-"+name)
			if got.direction != connector.DirectionInbound || got.colleague != nil {
				t.Errorf("an unauthenticated copy was read as the colleague's (direction %q, colleague %v)", got.direction, got.colleague)
			}
		})
	}
}

// An invitation a colleague organised is received mail: whom they invited is
// not whom they wrote to. It stays inbound, but its From names the colleague,
// so no reader credits it to the other side. Invited seats count.
func TestAColleaguesInvitationIsCreditedToTheColleague(t *testing.T) {
	ctx, db := bootstrapInternalMailWorkspace(t, "acme.com")
	ws, _ := principal.WorkspaceID(ctx)
	seat := seedSeat(ctx, t, db, "rep@acme.com", "active")
	colleague := seedSeat(ctx, t, db, "sam@acme.com", "invited")
	rec := colleagueCopy("invite-1", "sam@acme.com")
	fields, ok := rec.Fields.(capture.ActivityFields)
	if !ok {
		t.Fatalf("the fixture carries %T, want capture.ActivityFields", rec.Fields)
	}
	fields.HasCalendarPart = true
	rec.Fields = fields

	for range 2 {
		if _, err := capture.NewSink(db).Upsert(capturedAs(ctx, ws, seat, rec)); err != nil {
			t.Fatalf("Upsert: %v", err)
		}
	}

	got := storedMailFor(ctx, t, db, "invite-1")
	if got.direction != connector.DirectionInbound || got.colleague != nil {
		t.Errorf("stored as %s with sent_by_colleague %v, want the received invitation it is", got.direction, got.colleague)
	}
	if got.fromUser == nil || *got.fromUser != colleague {
		t.Errorf("the From participant is %v, want the colleague %v who organised it", got.fromUser, colleague)
	}

	unauth := colleagueCopy("invite-unauth", "sam@acme.com")
	unauth.SenderAuthentication = connector.SenderAuthentication{}
	unauth.Fields = fields
	if _, err := capture.NewSink(db).Upsert(capturedAs(ctx, ws, seat, unauth)); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if got := storedMailFor(ctx, t, db, "invite-unauth"); got.fromUser != nil {
		t.Errorf("an unauthenticated invitation credited seat %v; a sender types From", got.fromUser)
	}
}

// capturedAs is rec as the seat's own connector hands it to the sink: the
// acting connector is the one the record names.
func capturedAs(ctx context.Context, ws, seat ids.UUID, rec connector.NormalizedRecord) (context.Context, connector.NormalizedRecord) {
	ctx = sinkContextFor(ctx, ws, seat, seat)
	actor, _ := principal.Actor(ctx)
	actor.ID = rec.CapturedBy
	return principal.WithActor(ctx, actor), rec
}
