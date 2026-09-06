// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A group chat's roster against a real database: who was in the room, and what
// being in the room is worth.
//
// The load-bearing claim is a NEGATIVE one. An account resolves a party to a
// contact the installation already holds and to nothing else — no fact in this
// database attests that a channel account belongs to a member, so a roster
// grants nobody a read. Everything below either shows the record being made or
// shows the grant not being made.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// rosterProvider is a registered per-member transport, which is what a group
// chat arrives on. The account ids below are only meaningful against it.
const rosterProvider = "telegram"

// seedChatMessage records one captured channel message to hang a roster off.
func seedChatMessage(t *testing.T) ids.ActivityID {
	t.Helper()
	id := SeedIDRow(t, OwnerConn(t), `INSERT INTO activity (id, kind, subject, occurred_at, direction, source, captured_by, channel_provider)
		VALUES ($1, 'message', 'the group', '2026-09-01T09:00:00Z', 'inbound', 'telegram', 'connector:telegram', '`+rosterProvider+`')`)
	return ids.From[ids.ActivityKind](id)
}

// stampRoster runs the real capture write for a roster on a chat transport.
func stampRoster(t *testing.T, e *Env, activity ids.ActivityID, attested bool, parties ...connector.MessageParticipant) {
	t.Helper()
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		return capture.StampFurtherParticipants(ctx, tx, activity, "message", rosterProvider, attested, parties)
	}); err != nil {
		t.Fatalf("StampFurtherParticipants: %v", err)
	}
}

// rosterRow reads back one party stamped by account.
func rosterRow(t *testing.T, activity ids.ActivityID, account string) (userID, personID *ids.UUID, address *string, found bool) {
	t.Helper()
	err := OwnerConn(t).QueryRow(context.Background(), `
		SELECT user_id, person_id, address FROM activity_participant
		 WHERE activity_id = $1 AND channel_user_id = $2`, activity, account).Scan(&userID, &personID, &address)
	if err == pgx.ErrNoRows {
		return nil, nil, nil, false
	}
	if err != nil {
		t.Fatalf("reading the roster row: %v", err)
	}
	return userID, personID, address, true
}

// An attendee with an account and no address anywhere is the case a chat has
// and mail does not — the third human in a group. Before the account column
// they were dropped outright, so a four-person conversation was recorded as a
// two-person one.
func TestAPartyNamedOnlyByAccountIsRecorded(t *testing.T) {
	e := Setup(t)
	activity := seedChatMessage(t)

	stampRoster(t, e, activity, false,
		connector.MessageParticipant{ChannelUserID: "acct-51", DisplayName: "Sam Okonkwo", Role: connector.ParticipantRoleAttendee})

	userID, personID, address, found := rosterRow(t, activity, "acct-51")
	if !found {
		t.Fatal("a party the transport named by account was dropped; being in the room is a fact about the conversation")
	}
	if address != nil {
		t.Errorf("the row carries address %q, and the transport gave none", *address)
	}
	if userID != nil || personID != nil {
		t.Error("an account nobody has a binding for resolved to somebody")
	}
}

// The one resolution an account CAN do: person_channel_identity already binds
// the account to the outside human, and that binding is what the interaction
// graph joins on.
func TestAnAccountResolvesToTheContactItIsBoundTo(t *testing.T) {
	e := Setup(t)
	owner := OwnerConn(t)
	activity := seedChatMessage(t)
	person := e.SeedPerson(t, "Priya Raman", &e.Rep1)
	SeedIDRow(t, owner, `INSERT INTO person_channel_identity (id, person_id, provider, channel_user_id, source, captured_by)
		VALUES ($1, '`+person.String()+`', '`+rosterProvider+`', 'acct-77', 'capture', 'connector:telegram')`)

	stampRoster(t, e, activity, false,
		connector.MessageParticipant{ChannelUserID: "acct-77", Role: connector.ParticipantRoleAttendee})

	_, personID, _, found := rosterRow(t, activity, "acct-77")
	if !found {
		t.Fatal("the bound contact was not recorded at all")
	}
	if personID == nil || *personID != person {
		t.Error("an account the installation has a person binding for did not resolve to them")
	}
}

// A binding on ANOTHER transport is not this party. An account id is only
// meaningful against the provider that issued it, and two providers reusing a
// short numeric id is the ordinary case rather than the exotic one.
func TestAnAccountDoesNotResolveAcrossTransports(t *testing.T) {
	e := Setup(t)
	owner := OwnerConn(t)
	activity := seedChatMessage(t)
	person := e.SeedPerson(t, "Somebody Else", &e.Rep1)
	SeedIDRow(t, owner, `INSERT INTO person_channel_identity (id, person_id, provider, channel_user_id, source, captured_by)
		VALUES ($1, '`+person.String()+`', 'whatsapp', 'acct-77', 'capture', 'connector:whatsapp')`)

	stampRoster(t, e, activity, false,
		connector.MessageParticipant{ChannelUserID: "acct-77", Role: connector.ParticipantRoleAttendee})

	_, personID, _, found := rosterRow(t, activity, "acct-77")
	if !found {
		t.Fatal("the party was not recorded at all")
	}
	if personID != nil {
		t.Error("an account resolved to a person bound to it on a DIFFERENT transport — the id is the provider's, not a global name")
	}
}

// THE GRANT THAT IS NOT MADE, and the reason the arms above did not have to
// change. Even attested — the strongest a party list ever gets — an account
// never reaches a seat, because no fact in this database attests one to a
// member. A roster that could would let whoever writes it name a colleague and
// hand them a read of the conversation.
func TestAnAccountNeverBindsASeatEvenWhenAttested(t *testing.T) {
	e := Setup(t)
	owner := OwnerConn(t)
	activity := seedChatMessage(t)

	var colleagueEmail string
	if err := owner.QueryRow(context.Background(),
		`SELECT email FROM app_user WHERE id = $1`, e.Rep2).Scan(&colleagueEmail); err != nil {
		t.Fatalf("reading the colleague's address: %v", err)
	}

	stampRoster(t, e, activity, true,
		connector.MessageParticipant{ChannelUserID: colleagueEmail, Role: connector.ParticipantRoleAttendee},
		connector.MessageParticipant{ChannelUserID: "acct-51", Role: connector.ParticipantRoleAttendee})

	// The colleague's own address, spelled as an account, is the sharpest form
	// of the attempt: it is the exact string the address arm would have matched.
	userID, _, _, found := rosterRow(t, activity, colleagueEmail)
	if !found {
		t.Fatal("the party was not recorded at all")
	}
	if userID != nil {
		t.Fatal("an account bound a colleague's seat — a roster is a remote system's text, and this is the read it must not hand out")
	}
}

// The discovery gate reads `address IS NOT NULL` as its evidence that the
// PROVIDER enumerated a party, and a roster row must not satisfy it. This is
// that arm asked directly of the rows the stamp writes: nothing it produced can
// make an unrelated seat discover the conversation.
func TestARosterRowIsNotDiscoveryEvidence(t *testing.T) {
	e := Setup(t)
	activity := seedChatMessage(t)

	stampRoster(t, e, activity, true,
		connector.MessageParticipant{ChannelUserID: "acct-51", Role: connector.ParticipantRoleAttendee},
		connector.MessageParticipant{ChannelUserID: "acct-77", Role: connector.ParticipantRoleAttendee})

	// Counted together so the assertion cannot pass VACUOUSLY. A stamp that
	// wrote nothing at all satisfies "no evidence rows" perfectly, and would go
	// on satisfying it after the roster stopped landing entirely.
	var written, evidence int
	if err := OwnerConn(t).QueryRow(context.Background(), `
		SELECT count(*),
		       count(*) FILTER (WHERE user_id IS NOT NULL AND address IS NOT NULL)
		  FROM activity_participant WHERE activity_id = $1`, activity).Scan(&written, &evidence); err != nil {
		t.Fatalf("counting: %v", err)
	}
	if written != 2 {
		t.Fatalf("the roster left %d row(s), want 2 — with none written there is no evidence claim to test", written)
	}
	if evidence != 0 {
		t.Fatalf("a roster left %d row(s) that satisfy the discovery arm; it admits a seat on exactly `user_id = me AND address IS NOT NULL`", evidence)
	}
}

// Two parties who differ ONLY by account are two people, and the uniqueness key
// has to say so. Under the pre-account key they are one row and ON CONFLICT DO
// NOTHING keeps whichever arrived first — a silent loss with no error anywhere,
// which is the failure this assertion exists to catch.
func TestTwoPartiesDifferingOnlyByAccountAreTwoRows(t *testing.T) {
	e := Setup(t)
	activity := seedChatMessage(t)

	stampRoster(t, e, activity, false,
		connector.MessageParticipant{ChannelUserID: "acct-51", Role: connector.ParticipantRoleAttendee},
		connector.MessageParticipant{ChannelUserID: "acct-77", Role: connector.ParticipantRoleAttendee},
		connector.MessageParticipant{ChannelUserID: "acct-93", Role: connector.ParticipantRoleAttendee})

	var rows int
	if err := OwnerConn(t).QueryRow(context.Background(),
		`SELECT count(*) FROM activity_participant WHERE activity_id = $1`, activity).Scan(&rows); err != nil {
		t.Fatalf("counting: %v", err)
	}
	if rows != 3 {
		t.Fatalf("a three-person roster left %d row(s); a group recorded short is a group recorded wrong", rows)
	}
}

// Capture's sync loop is at-least-once, so a redelivered group message must add
// nothing — the same obligation the address rows are already under.
func TestStampingTheSameRosterTwiceAddsNothing(t *testing.T) {
	e := Setup(t)
	activity := seedChatMessage(t)
	parties := []connector.MessageParticipant{
		{ChannelUserID: "acct-51", Role: connector.ParticipantRoleAttendee},
		{ChannelUserID: "acct-77", Email: "legal@example.net", Role: connector.ParticipantRoleAttendee},
	}
	stampRoster(t, e, activity, false, parties...)
	stampRoster(t, e, activity, false, parties...)

	var rows int
	if err := OwnerConn(t).QueryRow(context.Background(),
		`SELECT count(*) FROM activity_participant WHERE activity_id = $1`, activity).Scan(&rows); err != nil {
		t.Fatalf("counting: %v", err)
	}
	if rows != len(parties) {
		t.Errorf("a redelivered group message left %d participant rows, want %d", rows, len(parties))
	}
}

// A party carrying BOTH keeps both, because the row records what was actually
// seen. The address is what person_email and the graph read; the account is what
// a reply routes on.
func TestAPartyWithBothKeepsBoth(t *testing.T) {
	e := Setup(t)
	activity := seedChatMessage(t)

	stampRoster(t, e, activity, false,
		connector.MessageParticipant{ChannelUserID: "acct-77", Email: "Legal@Example.NET", Role: connector.ParticipantRoleAttendee})

	_, _, address, found := rosterRow(t, activity, "acct-77")
	if !found {
		t.Fatal("the party was not recorded")
	}
	// Lower-cased, because that is how person_email stores an address and a case
	// difference would otherwise read as a different human.
	if address == nil || *address != "legal@example.net" {
		t.Errorf("the row's address is %v, want the lower-cased address the transport gave", address)
	}
}
