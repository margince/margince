// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose_test

// Who a hand-logged conversation says was in it.
//
// A LINK IS NOT A CONVERSATION. The links say which records a message is
// about; the participants say two people spoke, and everything network-shaped
// — the interaction and contact edges, the person graph's arms, who-knows, the
// decay lane — derives from the second. The capture path's stamping has its own
// suite (participants_integration_test.go); this is the hand-logged half, which
// is the one every seeded and manually recorded conversation rides.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type loggedParty struct {
	role   string
	user   *ids.UUID
	person *ids.UUID
}

// partiesOn reads the participant rows one activity carries, in a stable order.
func partiesOn(t *testing.T, e *integration.Env, activity ids.UUID) []loggedParty {
	t.Helper()
	var out []loggedParty
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(context.Background(), `
			SELECT role, user_id, person_id FROM activity_participant
			 WHERE activity_id = $1
			 ORDER BY role, coalesce(user_id, person_id)`, activity)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var party loggedParty
			if err := rows.Scan(&party.role, &party.user, &party.person); err != nil {
				return err
			}
			out = append(out, party)
		}
		return rows.Err()
	}); err != nil {
		t.Fatalf("reading the parties: %v", err)
	}
	return out
}

// logMail records one hand-logged email against the named people, as the API
// does: the request shape is the contract's, mapped through the same
// LogActivityInputFrom every transport uses.
func logMail(ctx context.Context, t *testing.T, e *integration.Env, kind crmcontracts.CreateActivityRequestKind,
	direction crmcontracts.CreateActivityRequestDirection, people ...ids.UUID,
) ids.UUID {
	t.Helper()
	subject := "Renewal terms"
	// Named once, used twice. The contract generates this as an ANONYMOUS struct,
	// so a caller must spell its shape out — and spelling it out at both the
	// make and the append is two copies of a shape the generator owns, which
	// drift apart the day a field is added to it.
	type mailLink = struct {
		EntityId   openapi_types.UUID                                `json:"entity_id"` //nolint:staticcheck // mirrors the generated inline struct, whose field is spelled EntityId
		EntityType crmcontracts.CreateActivityRequestLinksEntityType `json:"entity_type"`
	}
	links := make([]mailLink, 0, len(people))
	for _, person := range people {
		links = append(links, mailLink{
			EntityId:   openapi_types.UUID(person),
			EntityType: crmcontracts.CreateActivityRequestLinksEntityTypePerson,
		})
	}
	in, err := activities.LogActivityInputFrom(crmcontracts.CreateActivityRequest{
		Kind: kind, Subject: &subject, Direction: &direction, Links: &links, Source: "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	act, _, err := activities.NewStore(e.DB()).LogActivity(ctx, in)
	if err != nil {
		t.Fatalf("logging the %s: %v", kind, err)
	}
	return ids.UUID(act.Id)
}

func loggingRep(e *integration.Env) context.Context {
	return e.As(e.Rep1, []ids.UUID{e.Team1}, principal.Permissions{
		RoleKeys: []string{"rep"},
		Objects: map[string]principal.ObjectGrant{
			"activity": {Create: true, Read: true},
			"person":   {Read: true},
		},
		RowScope: principal.RowScopeAll,
	})
}

// EVERY person on the message, not the first. A thread with two contacts on it
// is where the contact-to-contact edge and the person graph's peer arm come
// from, and a stamping that took one counterparty would draw neither while
// every other surface still looked right.
func TestAHandLoggedMailNamesTheRepAndEveryContactOnIt(t *testing.T) {
	e := integration.Setup(t)
	ctx := loggingRep(e)
	alice := e.SeedPerson(t, "Alice Müller", &e.Rep1)
	bob := e.SeedPerson(t, "Bob Schmidt", &e.Rep1)

	activity := logMail(ctx, t, e, crmcontracts.CreateActivityRequestKindEmail,
		crmcontracts.CreateActivityRequestDirectionOutbound, alice, bob)

	parties := partiesOn(t, e, activity)
	if len(parties) != 3 {
		t.Fatalf("a mail to two contacts carries %d parties, want 3 (the rep and both of them): %+v",
			len(parties), parties)
	}
	// BOTH, BY NAME. Counting two person rows would pass a stamping that wrote
	// one contact twice, or one of them and somebody else — and the pair is the
	// whole reason a thread carries two people.
	var ours int
	named := map[ids.UUID]string{}
	for _, party := range parties {
		switch {
		case party.user != nil && *party.user == e.Rep1 && party.role == "from":
			ours++
		case party.person != nil:
			named[*party.person] = party.role
		}
	}
	if ours != 1 {
		t.Errorf("the rep who logged the mail is not on it as its sender: %+v", parties)
	}
	for who, name := range map[ids.UUID]string{alice: "Alice", bob: "Bob"} {
		role, on := named[who]
		if !on {
			t.Errorf("%s is linked to the mail and not on it: %+v", name, parties)
			continue
		}
		if role != "to" {
			t.Errorf("%s holds role %q on an outbound mail, want to", name, role)
		}
	}
}

// The roles MIRROR the direction, because the fold reads them: our side sends
// on outbound and receives on inbound, and a stamping that always wrote "from"
// for the rep would make every conversation look one-directional — which is
// what the strength score's reciprocity term is computed from.
func TestAnInboundMailPutsTheContactOnItAsTheSender(t *testing.T) {
	e := integration.Setup(t)
	ctx := loggingRep(e)
	alice := e.SeedPerson(t, "Alice Müller", &e.Rep1)

	activity := logMail(ctx, t, e, crmcontracts.CreateActivityRequestKindEmail,
		crmcontracts.CreateActivityRequestDirectionInbound, alice)

	// COUNTED, not merely walked. A loop over the rows accepts an empty result,
	// so a stamping that wrote nothing at all would pass a check about roles
	// without ever comparing one.
	parties := partiesOn(t, e, activity)
	if len(parties) != 2 {
		t.Fatalf("an inbound mail from one contact carries %d parties, want 2 (them and the rep): %+v",
			len(parties), parties)
	}
	var contact, rep *loggedParty
	for i := range parties {
		switch {
		case parties[i].person != nil && *parties[i].person == alice:
			contact = &parties[i]
		case parties[i].user != nil && *parties[i].user == e.Rep1:
			rep = &parties[i]
		}
	}
	if contact == nil || rep == nil {
		t.Fatalf("the mail names %+v, want the contact it came from and the rep who logged it", parties)
	}
	if contact.role != "from" {
		t.Errorf("the contact on an INBOUND mail holds role %q, want from — the fold reads the roles, "+
			"and a conversation stamped one-directional scores as one", contact.role)
	}
	if rep.role != "to" {
		t.Errorf("the rep on an INBOUND mail holds role %q, want to", rep.role)
	}
}

// A NOTE IS NOT A CONVERSATION. It is a record of somebody thinking, and
// counting it would let a rep's own notebook score as a relationship — which
// is why the interaction kinds are a closed set rather than "anything linked".
func TestANoteAboutSomebodyNamesNobodyAsHavingSpoken(t *testing.T) {
	e := integration.Setup(t)
	ctx := loggingRep(e)
	alice := e.SeedPerson(t, "Alice Müller", &e.Rep1)

	activity := logMail(ctx, t, e, crmcontracts.CreateActivityRequestKindNote,
		crmcontracts.CreateActivityRequestDirectionOutbound, alice)

	if parties := partiesOn(t, e, activity); len(parties) != 0 {
		t.Errorf("a note carries %d participant rows, want none: %+v", len(parties), parties)
	}
}
