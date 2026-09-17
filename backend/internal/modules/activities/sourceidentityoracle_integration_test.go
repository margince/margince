// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

// A Message-ID must not become an oracle.
//
// The identity table makes one message one activity, and the value it keys on
// is typed by whoever sent the message. So a caller can GUESS one. What they
// must never learn from guessing is whether somebody here already holds it —
// that is the existence of a colleague's mail, and a status code is enough to
// disclose it.
//
// The rule these tests pin: a create that loses the claim looks exactly like a
// create that had no identity to claim. Same status, same shape, their own row.

import (
	"context"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// importedEmail is the create an importer sends: its own provenance, and the
// message's own identity.
func importedEmail(sourceID, messageID string) LogActivityInput {
	direction := "inbound"
	from := "them@example.test"
	system := "hubspot"
	id := sourceID
	return LogActivityInput{
		Kind:         string(crmcontracts.ActivityKindEmail),
		Source:       "hubspot_import:145347700",
		SourceSystem: &system,
		SourceID:     &id,
		Direction:    &direction,
		EmailFrom:    from,
		EmailTo:      []string{"us@example.test"},
		RFCMessageID: messageID,
	}
}

func TestAGuessedMessageIDTellsTheGuesserNothing(t *testing.T) {
	e := setupFacts(t)
	ctx := context.Background()

	// A colleague's message, already here under a Message-ID a guesser is about
	// to name. Seeded through the real writer, as the seat who owns it.
	owner := ids.NewV7()
	e.exec(t, `INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Owner')`,
		owner, "owner-"+owner.String()+"@oracle.test")
	const contested = "contested@example.test"
	if _, _, err := e.store.LogActivity(seatCtx(e, owner), importedEmail("theirs-1", contested)); err != nil {
		t.Fatalf("seeding the colleague's message: %v", err)
	}

	// A different seat names the same Message-ID, and an unrelated one.
	guesser := ids.NewV7()
	e.exec(t, `INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Guesser')`,
		guesser, "guesser-"+guesser.String()+"@oracle.test")

	guessed, guessedCreated, err := e.store.LogActivity(
		seatCtx(e, guesser), importedEmail("mine-1", contested))
	if err != nil {
		t.Fatalf("a guessed Message-ID must not fail the write — that failure IS the oracle: %v", err)
	}
	free, freeCreated, err := e.store.LogActivity(
		seatCtx(e, guesser), importedEmail("mine-2", "free@example.test"))
	if err != nil {
		t.Fatalf("an unclaimed Message-ID: %v", err)
	}

	// The two answers must be indistinguishable. If a guesser can tell these
	// apart, they can enumerate which Message-IDs this installation holds.
	if guessedCreated != freeCreated {
		t.Errorf("created = %v for a claimed Message-ID and %v for a free one; "+
			"the difference tells a guesser which one somebody here already holds",
			guessedCreated, freeCreated)
	}
	if !guessedCreated {
		t.Error("the guesser's own row was not created — their message has its own " +
			"source key and belongs to them whatever else holds the identity")
	}
	if guessed.Id == free.Id {
		t.Fatal("both creates returned one activity")
	}

	// And the guess must not have reached the colleague's row.
	var holder ids.UUID
	if err := e.owner.QueryRow(ctx,
		`SELECT activity_id FROM activity_identity WHERE identity_kind = $1 AND identity_key = $2`,
		IdentityKindMail, contested).Scan(&holder); err != nil {
		t.Fatalf("reading who holds the contested identity: %v", err)
	}
	if holder == ids.UUID(guessed.Id) {
		t.Error("the guesser took the identity off the colleague who already held it")
	}
}

// The same seat's own two rows DO bind: that is the case the feature exists for
// — an import and a mailbox sync of one message, by one colleague.
func TestOneSeatsOwnImportAndCaptureAreOneMessage(t *testing.T) {
	e := setupFacts(t)

	seat := ids.NewV7()
	e.exec(t, `INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Seat')`,
		seat, "seat-"+seat.String()+"@oracle.test")
	ctx := seatCtx(e, seat)

	const shared = "shared@example.test"
	first, created, err := e.store.LogActivity(ctx, importedEmail("import-1", shared))
	if err != nil || !created {
		t.Fatalf("the first arrival: %v (created=%v)", err, created)
	}
	second, created, err := e.store.LogActivity(ctx, importedEmail("import-2", shared))
	if err != nil {
		t.Fatalf("the second arrival: %v", err)
	}
	if created {
		t.Error("the second arrival created a row; one message is one activity")
	}
	if second.Id != first.Id {
		t.Errorf("the second arrival returned %v, want the message already here (%v)", second.Id, first.Id)
	}
}

// seatCtx is one colleague acting, with authority to log and read activities.
func seatCtx(e *factsEnv, userID ids.UUID) context.Context {
	return principal.WithActor(
		principal.WithCorrelationID(principal.WithWorkspaceID(context.Background(), e.ws), ids.NewV7()),
		principal.Principal{
			Type: principal.PrincipalHuman, ID: "human:" + userID.String(), UserID: userID,
			Permissions: principal.Permissions{
				RoleKeys: []string{"rep"},
				Objects:  map[string]principal.ObjectGrant{"activity": {Read: true, Create: true}},
				RowScope: principal.RowScopeAll,
			},
		})
}
