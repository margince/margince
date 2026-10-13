// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// Who a held thread is with. The parties are message content, so they pass the
// same gate as the subject. Colleagues stay off the list, and a contact is
// named only where the reader may open it.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// seedHeldMessage writes a message captured by `capturer` under `audience` and
// a held verdict of `holder` raised about it.
func seedHeldMessage(t *testing.T, e *integration.Env, holder, capturer ids.UUID, threadKey, audience string) ids.UUID {
	t.Helper()
	activityID := ids.NewV7()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		ctx := context.Background()
		if _, err := tx.Exec(ctx, `
			INSERT INTO activity (id, kind, subject, occurred_at, source, captured_by, audience)
			VALUES ($1, 'email', 'Vertrag', now(), 'gmail', $2, $3)`,
			activityID, "human:"+capturer.String(), audience); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO capture_thread_verdict (thread_key, user_id, status, kind, attempts, first_activity_id)
			VALUES ($1, $2, 'held', 'legal', 1, $3)`, threadKey, holder, activityID)
		return err
	}); err != nil {
		t.Fatalf("seeding a held message: %v", err)
	}
	return activityID
}

func seedParty(t *testing.T, e *integration.Env, activityID ids.UUID, user, contact *ids.UUID, address, role string) {
	t.Helper()
	e.WsExec(t, `INSERT INTO activity_participant (activity_id, user_id, contact_id, address, role)
		VALUES ($1, $2, $3, $4, $5)`, activityID, user, contact, address, role)
}

func TestAHeldThreadNamesWhoOutsideTheWorkspaceItIsWith(t *testing.T) {
	e := integration.Setup(t)
	message := seedHeldMessage(t, e, e.Rep1, e.Rep1, "thread-parties", "workspace")
	colleague := e.WsScalar(t, `SELECT lower(email) FROM app_user WHERE id = $1`, e.Rep2)
	open := e.SeedContact(t, "Anwalt", nil)
	private := e.SeedContact(t, "Privat", nil)
	e.MakeCapturePrivate(t, "contact", private, e.Rep2)

	seedParty(t, e, message, &e.Rep1, nil, "", "to")
	seedParty(t, e, message, &e.Rep2, nil, colleague, "cc")
	seedParty(t, e, message, nil, nil, colleague, "bcc")
	seedParty(t, e, message, nil, &open, "anwalt@kanzlei.example", "from")
	seedParty(t, e, message, nil, &private, "privat@kanzlei.example", "to")
	seedParty(t, e, message, nil, nil, "anwalt@kanzlei.example", "cc")
	for _, address := range []string{"a@kanzlei.example", "b@kanzlei.example", "c@kanzlei.example", "d@kanzlei.example"} {
		seedParty(t, e, message, nil, nil, address, "cc")
	}

	got := heldThreads(t, e, e.Rep1)
	if len(got) != 1 {
		t.Fatalf("%d threads, want 1", len(got))
	}
	thread := got[0]
	if thread.ParticipantCount != 6 {
		t.Errorf("participant count = %d, want 6 external parties, colleagues and repeats left out", thread.ParticipantCount)
	}
	if len(thread.Participants) != 5 {
		t.Fatalf("%d participants shown, want 5", len(thread.Participants))
	}
	if first := thread.Participants[0]; first.Address != "anwalt@kanzlei.example" || first.ContactID == nil || *first.ContactID != open {
		t.Errorf("first party = %+v, want the sender with the contact the reader may open", first)
	}
	for _, party := range thread.Participants {
		if party.Address == colleague {
			t.Errorf("a colleague is listed among who a held thread is with")
		}
		if party.Address == "privat@kanzlei.example" && party.ContactID != nil {
			t.Errorf("a colleague's private contact was named on %s", party.Address)
		}
	}
}

func TestAHeldThreadNamesNobodyOnAMessageItsReaderMayNotRead(t *testing.T) {
	e := integration.Setup(t)
	message := seedHeldMessage(t, e, e.Rep1, e.Rep2, "thread-withheld", "participants")
	seedParty(t, e, message, nil, nil, "anwalt@kanzlei.example", "from")

	got := heldThreads(t, e, e.Rep1)
	if len(got) != 1 {
		t.Fatalf("%d threads, want 1", len(got))
	}
	if got[0].ParticipantCount != 0 || len(got[0].Participants) != 0 {
		t.Errorf("a withheld message named %d of %d parties, want none",
			len(got[0].Participants), got[0].ParticipantCount)
	}
}

// The reader's own forwarding alias is not somebody the thread is with. A
// colleague's alias stays listed: leaving it out would say it is theirs. An
// archived contact is not named at all.
func TestAHeldThreadLeavesOutTheReadersOwnAliasAndArchivedContacts(t *testing.T) {
	e := integration.Setup(t)
	message := seedHeldMessage(t, e, e.Rep1, e.Rep1, "thread-alias", "workspace")
	identities := capture.NewOwnerIdentityStore(InstallationDB(e.Pool))
	for seat, alias := range map[ids.UUID]string{e.Rep1: "ich@privat.example", e.Rep2: "kollege@privat.example"} {
		if _, err := identities.Add(purgeCtx(e, seat), capture.IdentityKindAddress, alias); err != nil {
			t.Fatalf("declaring %s: %v", alias, err)
		}
	}
	archived := e.SeedContact(t, "Ehemalig", nil)
	e.WsExec(t, `UPDATE contact SET archived_at = now() WHERE id = $1`, archived)
	seedParty(t, e, message, nil, nil, "ich@privat.example", "to")
	seedParty(t, e, message, nil, nil, "kollege@privat.example", "cc")
	seedParty(t, e, message, nil, &archived, "ehemalig@kanzlei.example", "from")

	parties := map[string]*ids.UUID{}
	for _, party := range heldThreads(t, e, e.Rep1)[0].Participants {
		parties[party.Address] = party.ContactID
	}
	if _, listed := parties["ich@privat.example"]; listed {
		t.Error("the reader's own alias is listed as somebody the thread is with")
	}
	if _, listed := parties["kollege@privat.example"]; !listed {
		t.Error("a colleague's alias was left out, which tells the reader it is theirs")
	}
	if contact, listed := parties["ehemalig@kanzlei.example"]; !listed || contact != nil {
		t.Errorf("an archived contact is named (%v), want the address with no contact", contact)
	}
}
