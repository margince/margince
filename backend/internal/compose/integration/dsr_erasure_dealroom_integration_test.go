// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Erasing a subject who was invited into a Deal Room.
//
// A room seat is the one place a named outside contact is stored WITHOUT a
// contact row: the buyer is invited by address long before anybody decides they
// are a contact. Erasure resolves a subject through their contact row and their
// addresses, so a seat is reached only by the address match — and this suite is
// what says it stays reached. Every row here is written by the real writers
// (contacts.Store, deals.Store, dealrooms.Store) and erased by the real
// privacy.Eraser: hand-inserted rows would prove nothing about either.

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/dealrooms"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// roomErasureAdmin is the seat this suite acts as: enough to create the deal
// room and its participant, to write the contact, and to erase them. Spelled
// here rather than widened on the shared admin fixture, so a grant added for
// this suite cannot quietly widen every other one.
var roomErasureAdmin = principal.Permissions{
	RoleKeys: []string{"admin"},
	Objects: map[string]principal.ObjectGrant{
		"contact":   {Create: true, Read: true, Update: true, Delete: true},
		"deal":      {Create: true, Read: true, Update: true, Delete: true},
		"deal_room": {Create: true, Read: true, Update: true, Delete: true},
		"activity":  {Create: true, Read: true, Update: true, Delete: true},
	},
	RowScope: principal.RowScopeAll,
}

// buyerSeat is one erasable subject as the room knows them: the contact record
// erasure resolves, and the room seat carrying the same address.
type buyerSeat struct {
	contact ids.ContactID
	room    ids.DealRoomID
	seat    ids.DealRoomParticipantID
	email   string
	// credential is the invitation the buyer signs in with, kept so a test can
	// speak as them rather than writing their comment on their behalf.
	credential string
}

// seedBuyerInARoom creates a contact, a deal, a room on it, and a seat for that
// contact's address — each through the store that owns it.
func seedBuyerInARoom(t *testing.T, e *Env, email string) buyerSeat {
	t.Helper()
	ctx := e.As(e.AdminUser, nil, roomErasureAdmin)
	name := "Rita Reviewer"
	contact, err := contacts.NewStore(e.DB()).CreateContact(ctx, contacts.CreateContactInput{
		FullName: name, Source: "manual",
		Emails: []contacts.ContactEmailInput{{Email: email, EmailType: "work", IsPrimary: true}},
	})
	if err != nil {
		t.Fatalf("seeding the buyer's contact record: %v", err)
	}
	dealID := e.SeedWonDealLinkedTo(t)
	rooms := dealrooms.NewStore(e.DB())
	title := "Acme Expansion — Deal Room"
	room, err := rooms.CreateRoom(ctx, dealrooms.CreateRoomInput{
		DealID: ids.From[ids.DealKind](dealID), Title: title, Source: "manual",
	})
	if err != nil {
		t.Fatalf("seeding the deal room: %v", err)
	}
	roomID := ids.From[ids.DealRoomKind](ids.UUID(room.Id))
	invited, err := rooms.InviteParticipant(ctx, roomID, dealrooms.InviteInput{
		FullName: name, Email: email, Capability: "comment", Source: "manual",
	})
	if err != nil {
		t.Fatalf("seeding the buyer's seat: %v", err)
	}
	return buyerSeat{
		contact:    ids.From[ids.ContactKind](ids.UUID(contact.Id)),
		room:       roomID,
		seat:       ids.From[ids.DealRoomParticipantKind](ids.UUID(invited.Participant.Id)),
		email:      email,
		credential: invited.Credential,
	}
}

// buyerSays posts a comment AS the buyer, through the signed-in path a buyer
// actually uses: the credential is exchanged for a session and the thread is opened
// under it, so the row carries author_participant_id the way a real one does.
func buyerSays(t *testing.T, e *Env, buyer buyerSeat, body string) {
	t.Helper()
	rooms := dealrooms.NewStore(e.DB())
	ctx := e.As(e.AdminUser, nil, roomErasureAdmin)
	issued, err := rooms.ExchangeCredential(ctx, buyer.credential)
	if err != nil {
		t.Fatalf("exchanging the buyer's credential: %v", err)
	}
	sess, err := rooms.ResolveSession(ctx, issued.Token)
	if err != nil {
		t.Fatalf("resolving the buyer's session: %v", err)
	}
	if _, err := rooms.OpenBuyerThread(ctx, sess, dealrooms.OpenThreadInput{
		Body: body, Source: "manual",
	}); err != nil {
		t.Fatalf("posting the buyer's comment: %v", err)
	}
}

// readSeat returns the seat's stored name, address and revocation as they are
// on disk, past every read gate: the question is what the DATABASE still holds
// about an erased contact, not what an API chooses to show.
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

	if err := privacy.NewEraser(e.DB()).EraseContact(e.As(e.AdminUser, nil, roomErasureAdmin), seeded.contact.UUID, "an erasure request from the subject"); err != nil {
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
	// says WHEN this contact was here. Written through the real exchange rather
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

	if err := privacy.NewEraser(e.DB()).EraseContact(e.As(e.AdminUser, nil, roomErasureAdmin), seeded.contact.UUID, "an erasure request from the subject"); err != nil {
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

	if err := privacy.NewEraser(e.DB()).EraseContact(
		e.As(e.AdminUser, nil, roomErasureAdmin), seeded.contact.UUID,
		"an erasure request from the subject",
	); err != nil {
		t.Fatalf("erasing the subject: %v", err)
	}

	if n := e.WsCount(t,
		`SELECT count(*) FROM audit_log WHERE entity_type = 'deal_room_participant'
		   AND entity_id = $1 AND action = 'erase'`, seeded.seat); n != 1 {
		t.Errorf("the wiped seat carries %d erase row(s), want exactly 1: without it the "+
			"invitation's audit image still discloses the erased address", n)
	}
}

// The buyer's own words go when they do.
//
// anonymizeDealRoomSeats wipes the seat without deleting it, and a comment references
// that seat ON DELETE RESTRICT, so nothing in the cascade reached the text: the name
// came off the seat while what the contact actually wrote stayed, readable by every
// colleague with access to the room.
func TestErasingASubjectTakesTheCommentsTheyWroteWithThem(t *testing.T) {
	e := Setup(t)
	buyer := seedBuyerInARoom(t, e, "rita@reviewer.example")
	said := "We cannot accept clause 4 as drafted — Rita"
	buyerSays(t, e, buyer, said)
	// And a colleague's own comment in the same room, which must NOT go: the delete
	// is keyed on the seat, and a room-wide one would satisfy every assertion below
	// about the buyer while destroying the seller's record of their own negotiation.
	sellerSaid := "Clause 4 is standard for this term length"
	sellerSays(t, e, buyer.room, sellerSaid)

	if n := commentsBy(t, e, buyer.seat); n != 1 {
		t.Fatalf("the buyer's comment was not seeded (%d rows), so this test proves nothing", n)
	}

	if err := privacy.NewEraser(e.DB()).EraseContact(
		e.As(e.AdminUser, nil, roomErasureAdmin), buyer.contact.UUID, "subject request",
	); err != nil {
		t.Fatalf("EraseContact → %v", err)
	}

	if n := commentsBy(t, e, buyer.seat); n != 0 {
		t.Errorf("%d comment(s) the erased subject wrote are still in the room, signed by a seat "+
			"whose name was wiped — the erasure reported the data destroyed", n)
	}
	// By TEXT as well as by author, because a row kept under a different author
	// would answer the count above and still read out what they said.
	if n := e.WsCount(t, `SELECT count(*) FROM deal_room_comment WHERE body = $1`, said); n != 0 {
		t.Errorf("the erased subject's words are still stored verbatim (%d row(s))", n)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM deal_room_comment WHERE body = $1`, sellerSaid); n != 1 {
		t.Errorf("the colleague's own comment went with the subject's (%d left) — what the seller "+
			"wrote in their own negotiation is their record, not the erased contact's", n)
	}
}

// sellerSays posts a comment as the COLLEAGUE, through the seller-side opener: the row
// carries author_user_id, which is what makes it one the erasure must leave alone.
func sellerSays(t *testing.T, e *Env, room ids.DealRoomID, body string) {
	t.Helper()
	if _, err := dealrooms.NewStore(e.DB()).OpenThread(
		e.As(e.AdminUser, nil, roomErasureAdmin), room, dealrooms.OpenThreadInput{
			Body: body, Source: "manual",
		},
	); err != nil {
		t.Fatalf("posting the colleague's comment: %v", err)
	}
}

// commentsBy counts what one seat is on record as having written, read past every
// API gate: the question is what the database holds, not what a reader is shown.
func commentsBy(t *testing.T, e *Env, seat ids.DealRoomParticipantID) int {
	t.Helper()
	return e.WsCount(t, `SELECT count(*) FROM deal_room_comment WHERE author_participant_id = $1`, seat)
}

// The tombstone address cannot reach a contact record at all.
//
// This is where the erasure's own guard stops being reachable for new data: a seat is
// resolved by ADDRESS, so a subject holding the one an erasure writes would be
// indistinguishable from every seat already wiped. values.ParseEmail refuses it, so the
// state the eraser refuses can no longer be created — and the eraser keeps refusing it
// for rows written before that, which TestEraseDealRoomSeatsRefusesTheTombstoneAddress
// in the privacy package covers directly.
func TestTheTombstoneAddressCannotBeStoredOnAContact(t *testing.T) {
	e := Setup(t)
	ctx := e.As(e.AdminUser, nil, roomErasureAdmin)

	_, err := contacts.NewStore(e.DB()).CreateContact(ctx, contacts.CreateContactInput{
		FullName: "Rita Reviewer", Source: "manual",
		Emails: []contacts.ContactEmailInput{
			{Email: values.ErasedEmail, EmailType: "work", IsPrimary: true},
		},
	})
	var parse *values.ParseError
	if !errors.As(err, &parse) || parse.Code != "email_reserved" {
		t.Fatalf("CreateContact → %v, want the reserved-address refusal: any other failure lets "+
			"a change that stops rejecting the address pass this test, and such a subject's "+
			"seats are then indistinguishable from every seat already erased", err)
	}
}
