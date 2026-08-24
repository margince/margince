// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Erasing a subject who was invited into a Deal Room.
//
// A room seat is the one place a named outside person is stored WITHOUT a
// person row: the buyer is invited by address long before anybody decides they
// are a contact. Erasure resolves a subject through their person row and their
// addresses, so a seat is reached only by the address match — and this suite is
// what says it stays reached. Every row here is written by the real writers
// (people.Store, deals.Store, dealrooms.Store) and erased by the real
// privacy.Eraser: hand-inserted rows would prove nothing about either.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/gradionhq/margince/backend/internal/modules/dealrooms"
	"github.com/gradionhq/margince/backend/internal/modules/people"
	"github.com/gradionhq/margince/backend/internal/modules/privacy"
	"github.com/gradionhq/margince/backend/internal/platform/database"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/principal"
)

// roomErasureAdmin is the seat this suite acts as: enough to create the deal
// room and its participant, to write the contact, and to erase them. Spelled
// here rather than widened on the shared admin fixture, so a grant added for
// this suite cannot quietly widen every other one.
var roomErasureAdmin = principal.Permissions{
	RoleKeys: []string{"admin"},
	Objects: map[string]principal.ObjectGrant{
		"person":    {Create: true, Read: true, Update: true, Delete: true},
		"deal":      {Create: true, Read: true, Update: true, Delete: true},
		"deal_room": {Create: true, Read: true, Update: true, Delete: true},
		"activity":  {Create: true, Read: true, Update: true, Delete: true},
	},
	RowScope: principal.RowScopeAll,
}

// buyerSeat is one erasable subject as the room knows them: the contact record
// erasure resolves, and the room seat carrying the same address.
type buyerSeat struct {
	person ids.PersonID
	room   ids.DealRoomID
	seat   ids.DealRoomParticipantID
	email  string
}

// seedBuyerInARoom creates a contact, a deal, a room on it, and a seat for that
// contact's address — each through the store that owns it.
func seedBuyerInARoom(t *testing.T, e *Env, email string) buyerSeat {
	t.Helper()
	ctx := e.As(e.AdminUser, nil, roomErasureAdmin)
	name := "Rita Reviewer"
	person, err := people.NewStore(e.DB()).CreatePerson(ctx, people.CreatePersonInput{
		FullName: name, Source: "ui",
		Emails: []people.PersonEmailInput{{Email: email, EmailType: "work", IsPrimary: true}},
	})
	if err != nil {
		t.Fatalf("seeding the buyer's contact record: %v", err)
	}
	dealID := e.SeedWonDealLinkedTo(t)
	rooms := dealrooms.NewStore(e.DB())
	title := "Acme Expansion — Deal Room"
	room, err := rooms.CreateRoom(ctx, dealrooms.CreateRoomInput{
		DealID: ids.From[ids.DealKind](dealID), Title: title, Source: "ui",
	})
	if err != nil {
		t.Fatalf("seeding the deal room: %v", err)
	}
	roomID := ids.From[ids.DealRoomKind](ids.UUID(room.Id))
	invited, err := rooms.InviteParticipant(ctx, roomID, dealrooms.InviteInput{
		FullName: name, Email: email, Capability: "comment", Source: "ui",
	})
	if err != nil {
		t.Fatalf("seeding the buyer's seat: %v", err)
	}
	return buyerSeat{
		person: ids.From[ids.PersonKind](ids.UUID(person.Id)),
		room:   roomID,
		seat:   ids.From[ids.DealRoomParticipantKind](ids.UUID(invited.Participant.Id)),
		email:  email,
	}
}

// readSeat returns the seat's stored name, address and revocation as they are
// on disk, past every read gate: the question is what the DATABASE still holds
// about an erased person, not what an API chooses to show.
func readSeat(t *testing.T, e *Env, seat ids.DealRoomParticipantID) (name, email string, revoked bool) {
	t.Helper()
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		var revokedAt *string
		err := tx.QueryRow(ctx,
			`SELECT full_name, email, revoked_at::text FROM deal_room_participant WHERE id = $1`,
			seat).Scan(&name, &email, &revokedAt)
		revoked = revokedAt != nil
		return err
	}); err != nil {
		t.Fatalf("reading the seat back: %v", err)
	}
	return name, email, revoked
}

func TestErasingASubjectWipesTheDealRoomSeatCarryingTheirAddress(t *testing.T) {
	e := Setup(t)
	seeded := seedBuyerInARoom(t, e, "rita.erasure@acme.test")

	before, beforeEmail, beforeRevoked := readSeat(t, e, seeded.seat)
	if before != "Rita Reviewer" || beforeEmail != seeded.email || beforeRevoked {
		t.Fatalf("the seat did not start as a live, named seat: name=%q email=%q revoked=%v",
			before, beforeEmail, beforeRevoked)
	}

	if err := privacy.NewEraser(e.DB()).ErasePerson(e.As(e.AdminUser, nil, roomErasureAdmin), seeded.person.UUID, "an erasure request from the subject"); err != nil {
		t.Fatalf("erasing the subject: %v", err)
	}

	name, email, revoked := readSeat(t, e, seeded.seat)
	if name == "Rita Reviewer" {
		t.Errorf("the erased subject is still named on their Deal Room seat: %q", name)
	}
	if email == seeded.email {
		t.Errorf("the erased subject's address is still on their Deal Room seat: %q", email)
	}
	if !revoked {
		t.Error("the erased subject's seat still admits them: erasure left the access live")
	}
}

func TestErasingASubjectLeavesNoRoomActivityTrailBehind(t *testing.T) {
	e := Setup(t)
	seeded := seedBuyerInARoom(t, e, "trail.erasure@acme.test")

	// A sign-in is what the buyer's own door writes, and it is the row that
	// says WHEN this person was here. Written through the real exchange rather
	// than inserted, so the test cannot pass against a trail the product never
	// produces.
	rooms := dealrooms.NewStore(e.DB())
	issued, err := rooms.ResendInvitation(e.As(e.AdminUser, nil, roomErasureAdmin), seeded.room, seeded.seat)
	if err != nil {
		t.Fatalf("issuing the buyer's credential: %v", err)
	}
	if _, err := rooms.ExchangeCredential(context.Background(), issued.Credential); err != nil {
		t.Fatalf("the buyer signing in: %v", err)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM deal_room_engagement WHERE participant_id = $1`,
		seeded.seat); n == 0 {
		t.Fatal("signing in recorded nothing, so this test would pass against a product that records nothing")
	}

	if err := privacy.NewEraser(e.DB()).ErasePerson(e.As(e.AdminUser, nil, roomErasureAdmin), seeded.person.UUID, "an erasure request from the subject"); err != nil {
		t.Fatalf("erasing the subject: %v", err)
	}

	if n := e.WsCount(t, `SELECT count(*) FROM deal_room_engagement WHERE participant_id = $1`,
		seeded.seat); n != 0 {
		t.Errorf("erasure left %d engagement row(s): when the subject signed in is still recorded", n)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM deal_room_session WHERE participant_id = $1`,
		seeded.seat); n != 0 {
		t.Errorf("erasure left %d live session(s): the erased subject can still enter the room", n)
	}
}

// The invitation's own audit image stores the buyer's address in plain text,
// and the record-history read serves images verbatim. Without an erase row on
// the seat, the audit log hands the "erased" address straight back — an
// erasure the record itself contradicts.
func TestErasingASubjectTombstonesTheSeatSoTheAuditLogStopsAtIt(t *testing.T) {
	e := Setup(t)
	seeded := seedBuyerInARoom(t, e, "audit.erasure@acme.test")

	if n := e.WsCount(t,
		`SELECT count(*) FROM audit_log WHERE entity_type = 'deal_room_participant'
		   AND entity_id = $1 AND action = 'erase'`, seeded.seat); n != 0 {
		t.Fatalf("the seat carried %d erase row(s) before any erasure ran", n)
	}

	if err := privacy.NewEraser(e.DB()).ErasePerson(
		e.As(e.AdminUser, nil, roomErasureAdmin), seeded.person.UUID,
		"an erasure request from the subject"); err != nil {
		t.Fatalf("erasing the subject: %v", err)
	}

	if n := e.WsCount(t,
		`SELECT count(*) FROM audit_log WHERE entity_type = 'deal_room_participant'
		   AND entity_id = $1 AND action = 'erase'`, seeded.seat); n != 1 {
		t.Errorf("the wiped seat carries %d erase row(s), want exactly 1: without it the "+
			"invitation's audit image still discloses the erased address", n)
	}
}
